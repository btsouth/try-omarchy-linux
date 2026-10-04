#!/usr/bin/env python3
"""Check drop positions and pointer-moved reports in the patched QEMU SDL code."""
import argparse
import importlib.util
from pathlib import Path
import subprocess
import tempfile

spec = importlib.util.spec_from_file_location('sdl_focus', Path(__file__).with_name('test-sdl-focus.py'))
sdl_focus = importlib.util.module_from_spec(spec)
spec.loader.exec_module(sdl_focus)


def check(source):
    state = source[source.index('#define SDL_DROP_WATCH_MS'):source.index('/* Runs on the thread')]
    harness = r'''
#include <assert.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
typedef struct { int w, h; } SDL_Window;
typedef int QemuConsole;
typedef struct { uint32_t windowID; int x, y; } SDL_MouseMotionEvent;
typedef struct strList { struct strList *next; char *value; } strList;
struct sdl2_console { SDL_Window *real_window; struct { QemuConsole *con; } dcl; };
static uint64_t ticks = 1000;
static uint64_t SDL_GetTicks64(void) { return ticks; }
static void SDL_GetWindowSize(SDL_Window *w, int *x, int *y) { *x = w->w; *y = w->h; }
static int qemu_console_get_index(QemuConsole *c) { return *c; }
static int drops, moved;
static bool has_x, has_y, has_w, has_h;
static int64_t last_x, last_y, last_w, last_h, last_display;
static void qapi_event_send_display_file_drop(int64_t display, strList *files,
    bool hx, int64_t x, bool hy, int64_t y, bool hw, int64_t w, bool hh, int64_t h)
{
    drops++; last_display = display; has_x = hx; has_y = hy; has_w = hw; has_h = hh;
    last_x = x; last_y = y; last_w = w; last_h = h;
}
static void qapi_event_send_display_drop_pointer_moved(int64_t display) { moved++; }
''' + state + ''.join(sdl_focus.function(source, name) for name in (
        'sdl3_drop_watch', 'sdl_drop_pointer_moved', 'sdl_drop_pointer_left',
        'sdl_drop_report', 'sdl_drop_motion')) + r'''
static SDL_Window window = {800, 600};
static QemuConsole con = 0;
static struct sdl2_console console = {&window, {&con}};
static void sdl3(uint32_t type, uint32_t id, float x, float y)
{
    SDL3DropEvent e = {type, 0, 0, id, x, y};
    sdl3_drop_watch(NULL, &e);
}
static void motion(uint32_t id, int x, int y)
{
    SDL_MouseMotionEvent m = {id, x, y};
    sdl_drop_motion(&m);
}
static bool positioned(void) { return has_x && has_y && has_w && has_h; }
int main(void)
{
    strList files = {NULL, "/tmp/a"};
    /* SDL3's drop point for this window is reported with the window size. */
    sdl3(0x1004, 7, 100.5f, 200.f);
    sdl3(0x1003, 7, 120.7f, 210.f);
    sdl_drop_report(7, &console, &files);
    assert(drops == 1 && positioned() && last_x == 120 && last_y == 210 && last_w == 800 && last_h == 600);
    /* Jitter within 4 px is not a move; the first real move is reported once. */
    motion(7, 124, 206); assert(moved == 0);
    motion(7, 125, 210); assert(moved == 1);
    motion(7, 400, 400); assert(moved == 1);
    /* A point is used once: a later drop without one has no position. */
    sdl_drop_report(7, &console, &files); assert(drops == 2 && !positioned());
    /* Leaving the window or entering another one counts as a move. */
    sdl3(0x1003, 7, 10.f, 10.f); sdl_drop_report(7, &console, &files); assert(positioned());
    sdl_drop_pointer_left(8); assert(moved == 1);
    sdl_drop_pointer_left(7); assert(moved == 2);
    sdl3(0x1003, 7, 10.f, 10.f); sdl_drop_report(7, &console, &files);
    motion(9, 10, 10); assert(moved == 3);
    /* The watch ends after 30 seconds. */
    sdl3(0x1003, 7, 10.f, 10.f); sdl_drop_report(7, &console, &files);
    ticks += 30000; motion(7, 300, 300); assert(moved == 3);
    /* A point from another window, outside the window or from another event type is not used. */
    sdl3(0x1003, 8, 10.f, 10.f); sdl_drop_report(7, &console, &files); assert(!positioned());
    sdl3(0x1003, 7, 800.f, 10.f); sdl_drop_report(7, &console, &files); assert(!positioned());
    sdl3(0x400, 7, 10.f, 10.f); sdl_drop_report(7, &console, &files); assert(!positioned());
    /* Without a point, nothing is watched. */
    motion(7, 500, 500); assert(moved == 3);
    return 0;
}
'''
    with tempfile.TemporaryDirectory(prefix='tryomarchy-sdl-drop-') as tmp:
        root = Path(tmp)
        (root / 'test.c').write_text(harness)
        subprocess.run(['cc', '-std=gnu11', '-O2', '-Wall', '-Werror', str(root / 'test.c'),
                        '-o', str(root / 'test')], check=True)
        subprocess.run([str(root / 'test')], check=True)
    print('ok - drop positions, single use, pointer moved, leave, other window, expiry')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--manifest', type=Path)
    group.add_argument('--source', type=Path)
    args = parser.parse_args()
    if args.manifest:
        sdl_focus.check = check
        sdl_focus.from_manifest(args.manifest)
    else:
        check(args.source.read_text())
