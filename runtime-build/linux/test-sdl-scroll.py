#!/usr/bin/env python3
"""Test the Flatpak QEMU's SDL -> virtio-tablet guest ABI without a desktop.

Run in the GNOME SDK with the built app mounted at /app, for example:
flatpak build --filesystem=/src BUILD python3 /src/runtime-build/linux/test-sdl-scroll.py /app/bin/qemu-system-x86_64

An LD_PRELOAD shim injects SDL wheel events at SDL_PollEvent; QEMU's actual
handler, input routing and virtio transport write a diskless guest's queue.
The dummy SDL driver opens no window on a host display. No guest OS boots.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import shlex
import socket
import struct
import subprocess
import tempfile
import time
import unittest

spec = importlib.util.spec_from_file_location(
    'pinch_test', Path(__file__).parents[1] / 'test-virtio-pinch.py')
pinch = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pinch)


class SDLScrollTests(pinch.VirtioPinchTests):
    __unittest_skip__ = False
    test_touchpad_capabilities_and_contact_frames = None

    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix='linux-sdl-scroll-')
        self.addCleanup(self.directory.cleanup)
        work = Path(self.directory.name)
        shim = work / 'inject.so'
        flags = shlex.split(subprocess.check_output(
            ['pkg-config', '--cflags', '--libs', 'sdl2'], text=True))
        subprocess.run(['cc', '-shared', '-fPIC', '-Wall', '-Wextra', '-Werror',
                        str(Path(__file__).with_name('sdl-scroll-inject.c')),
                        '-o', str(shim), *flags, '-ldl'], check=True)
        self.events_file = work / 'events'
        self.log = open(work / 'qemu.log', 'w+')
        self.addCleanup(self.log.close)
        (work / 'bios').write_bytes(bytes(65536))
        addresses = []
        for _ in range(2):
            with socket.socket() as reservation:
                reservation.bind(('127.0.0.1', 0))
                addresses.append(reservation.getsockname())
        env = dict(os.environ, SDL_VIDEODRIVER='dummy', SDL_RENDER_DRIVER='software',
                   SDL_AUDIODRIVER='dummy', LD_PRELOAD=str(shim),
                   QEMU_SDL_SCROLL_EVENTS=str(self.events_file))
        self.process = subprocess.Popen([
            BINARY, '-machine', 'q35', '-accel', 'qtest', '-m', '64M',
            '-nodefaults', '-vga', 'std', '-display', 'sdl,gl=off', '-S',
            '-bios', str(work / 'bios'),
            '-device', 'virtio-tablet-pci,addr=03.0,romfile=',
            '-qtest', f'tcp:{addresses[0][0]}:{addresses[0][1]},server=on,wait=off',
            '-qmp', f'tcp:{addresses[1][0]}:{addresses[1][1]},server=on,wait=off',
        ], env=env, stdout=self.log, stderr=self.log)
        self.addCleanup(self.stop)
        self.qtest = self.connect(addresses[0])
        self.qmp = self.connect(addresses[1])
        json.loads(self.qmp.readline())
        self.command('qmp_capabilities')
        self.bar = 0x10000000
        cap = self.pci_read(0x34) & 0xff
        self.caps = {}
        seen = set()
        while cap:
            self.assertNotIn(cap, seen)
            seen.add(cap)
            header = self.pci_read(cap)
            if header & 0xff == 9 and header >> 24 in (1, 2, 3, 4):
                self.assertEqual(self.pci_read(cap + 4) & 0xff, 4)
                self.caps[header >> 24] = self.bar + self.pci_read(cap + 8)
            cap = (header >> 8) & 0xff
        self.pci_write(0x20, self.bar)
        self.pci_write(0x24, 0)
        self.pci_write(0x04, 6)
        self.assertIn(1, self.caps)
        self.assertIn(4, self.caps)

    def pci_read(self, offset):
        self.qt(f'outl 0xcf8 {0x80001800 + offset:#x}')
        return int(self.qt('inl 0xcfc'), 0)

    def pci_write(self, offset, value):
        self.qt(f'outl 0xcf8 {0x80001800 + offset:#x}')
        self.qt(f'outl 0xcfc {value:#x}')

    def test_precise_scroll(self):
        self.assertEqual(self.config(1).rstrip(b'\0'), b'QEMU Virtio Tablet')
        axes = int.from_bytes(self.config(0x11, 2), 'little')
        for axis in (6, 8, 11, 12):
            self.assertTrue(axes & (1 << axis))
        self.assertFalse(axes & 3)  # absolute tablet must not claim REL_X/Y
        common = self.caps[1]
        self.qt(f'writeb {common + 20:#x} 3')
        self.qt(f'writel {common + 8:#x} 1')
        self.qt(f'writel {common + 12:#x} 1')
        self.qt(f'writeb {common + 20:#x} 11')
        self.qt(f'writew {common + 22:#x} 0')
        self.qt(f'writew {common + 24:#x} 64')
        desc, avail, used, buffers = 0x100000, 0x101000, 0x102000, 0x103000
        self.write(desc, b''.join(struct.pack('<QIHH', buffers + i * 8, 8, 2, 0)
                                  for i in range(64)))
        self.write(avail, struct.pack('<66H', 0, 64, *range(64)))
        for offset, address in ((32, desc), (40, avail), (48, used)):
            self.qt(f'writeq {common + offset:#x} {address:#x}')
        self.qt(f'writew {common + 28:#x} 1')
        self.qt(f'writeb {common + 20:#x} 15')
        self.command('cont')

        def inject(x, y, flipped, expected):
            before = int(self.qt(f'readw {used + 2:#x}'), 0)
            pending = self.events_file.with_suffix('.pending')
            pending.write_text(f'{x} {y} {int(flipped)}\n')
            pending.replace(self.events_file)
            deadline = time.monotonic() + 5
            while int(self.qt(f'readw {used + 2:#x}'), 0) < before + len(expected) + 1:
                if time.monotonic() >= deadline:
                    self.log.flush()
                    self.log.seek(0)
                    self.fail('SDL event did not reach guest queue:\n' + self.log.read())
                time.sleep(0.01)
            count = int(self.qt(f'readw {used + 2:#x}'), 0)
            events = [struct.unpack('<HHi', self.read(buffers + i * 8, 8))
                      for i in range(before, count)]
            self.assertEqual(events, expected + [(0, 0, 0)])
            print(f'SDL ({x}, {y}), flipped={flipped} -> guest {events}', flush=True)

        # Integer SDL fields are zero for each sub-detent movement.
        inject(0.25, 0.125, False, [(2, 12, 30), (2, 11, 15)])
        inject(-0.25, -0.125, False, [(2, 12, -30), (2, 11, -15)])
        inject(0.25, 0.5, True, [(2, 12, -30), (2, 11, -60)])
        inject(-0.25, -0.5, True, [(2, 12, 30), (2, 11, 60)])
        # Whole notches retain legacy events; positive horizontal is right.
        inject(2, -3, False, [(2, 12, 240), (2, 6, 2), (2, 11, -360), (2, 8, -3)])
        # Sub-unit fractions accumulate rather than disappearing between frames.
        for value in (3, 4, 4, 4):
            inject(0, 0.03125, False, [(2, 11, value)])

        # QEMU 11.1's QAPI and inline input events must use the same wheel mask.
        before = int(self.qt(f'readw {used + 2:#x}'), 0)
        self.command('input-send-event', {'events': [
            {'type': 'rel', 'data': {'axis': 'wheel', 'value': 30}},
            {'type': 'rel', 'data': {'axis': 'hwheel', 'value': -30}},
        ]})
        count = int(self.qt(f'readw {used + 2:#x}'), 0)
        events = [struct.unpack('<HHi', self.read(buffers + i * 8, 8))
                  for i in range(before, count)]
        self.assertEqual(events, [(2, 11, 30), (2, 12, -30), (0, 0, 0)])
        print(f'QMP wheel routing -> guest {events}', flush=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('qemu', help='Linux-built QEMU with SDL and qtest support')
    BINARY = parser.parse_args().qemu
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(SDLScrollTests)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    raise SystemExit(not result.wasSuccessful())
