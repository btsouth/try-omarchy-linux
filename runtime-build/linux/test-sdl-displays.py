#!/usr/bin/env python3
"""Compile the manifest-patched SDL placement/fullscreen code with host mocks.

Optional --real-sdl also exercises an actual SDL dummy driver (no GUI/display).
The mock covers hotplug and multiple host outputs which dummy cannot provide.
"""
import argparse
import json
import os
import importlib.util
from pathlib import Path
import shlex
import subprocess
import tempfile

spec = importlib.util.spec_from_file_location('focus', Path(__file__).parents[1] / 'test-sdl-focus.py')
focus = importlib.util.module_from_spec(spec)
spec.loader.exec_module(focus)


def check(source, real_sdl=False):
    prefix = r'''
#include <assert.h>
#include <stdbool.h>
#include <stdlib.h>
#include <string.h>
#include <glib.h>
'''
    mock = r'''
typedef struct { int x,y,w,h; } SDL_Rect;
typedef struct { int display, fullscreen, flags; } SDL_Window;
#define SDL_WINDOWPOS_CENTERED_DISPLAY(n) (1000+(n))
#define SDL_WINDOW_FULLSCREEN_DESKTOP 1
#define SDL_WINDOW_INPUT_FOCUS 2
static int displays=2, positions, changes;
static const char *names[]={"DP-1","HDMI-A-1","DP-3"};
static SDL_Rect bounds[]={{0,0,1920,1080},{1920,0,1600,900},{3520,0,1280,720}};
static int SDL_GetDisplayBounds(int i,SDL_Rect *b) {if(i<0||i>=displays)return -1;*b=bounds[i];return 0;}
static int SDL_GetNumVideoDisplays(void) { return displays; }
static const char *SDL_GetDisplayName(int i) { return names[i]; }
static void SDL_SetWindowPosition(SDL_Window *w,int x,int y) { assert(x==y);w->display=x-1000;positions++; }
static int SDL_SetWindowFullscreen(SDL_Window *w,int f) {w->fullscreen=f;changes++;return 0;}
static int SDL_GetWindowFlags(SDL_Window *w) {return w->flags;}
static void SDL_SetWindowMouseGrab(SDL_Window *w,int f) {}
#define SDL_FALSE 0
'''
    state = r'''
struct sdl2_console {SDL_Window *real_window;int idx;char *target_monitor;
 SDL_Rect target_bounds;bool has_target_bounds;
 bool fullscreen,fullscreen_requested;int saved_grab;};
static struct sdl2_console consoles[4], *sdl2_console=consoles;
static int sdl2_num_outputs=4, gui_grab;
static bool focus_keyboard_grab,absolute_enabled;
static bool initial_fullscreen;
static bool qemu_input_is_absolute(void *p) {return true;}
static void sdl_grab_start(struct sdl2_console *s) {}
static void sdl_grab_end(struct sdl2_console *s) {}
static void sdl2_redraw(struct sdl2_console *s) {}
#define error_report(...) ((void)0)
'''
    # toggle references dcl.con in its input policy.
    state = state.replace('int saved_grab;}', 'int saved_grab;struct {void *con;} dcl;}')
    functions = ''.join(focus.function(source, name) for name in (
        'sdl_target_display', 'sdl_pin_output', 'sdl_display_available', 'sdl_set_fullscreen',
        'sdl_configure_output', 'toggle_full_screen', 'sdl_host_displays_changed'))
    tests = r'''
int main(void) {
 SDL_Window windows[4]={0};
 for (int i=0;i<4;i++) {consoles[i].idx=i;consoles[i].real_window=&windows[i];sdl_configure_output(&consoles[i]);}
 assert(!consoles[0].fullscreen_requested);
 setenv("QEMU_SDL_OUTPUT_0","HDMI-A-1",1);
 setenv("QEMU_SDL_OUTPUT_FULLSCREEN_0","true",1);
 sdl_configure_output(&consoles[0]);
 assert(consoles[0].fullscreen_requested && sdl_target_display(&consoles[0])==1);
 consoles[1].target_monitor="DP-1";consoles[2].target_monitor="HDMI-A-1";
 consoles[3].target_monitor="missing";
 toggle_full_screen(&consoles[0]);toggle_full_screen(&consoles[1]);
 assert(consoles[0].fullscreen && consoles[1].fullscreen);
 assert(windows[0].display==1 && windows[1].display==0);
 toggle_full_screen(&consoles[2]);toggle_full_screen(&consoles[3]);
 assert(!consoles[2].fullscreen && !consoles[3].fullscreen);
 /* Independent hotkey: output 0 changes without altering output 1. */
 toggle_full_screen(&consoles[0]);assert(!consoles[0].fullscreen && consoles[1].fullscreen);
 toggle_full_screen(&consoles[2]);assert(consoles[2].fullscreen);
 /* Reorder display indices: fullscreen still follows the same connectors. */
 names[0]="HDMI-A-1";names[1]="DP-1";sdl_host_displays_changed();
 assert(windows[1].display==1 && windows[2].display==0);
 /* Unplug DP-1, window survives on primary; replug never steals fullscreen. */
 displays=1;sdl_host_displays_changed();assert(!consoles[1].fullscreen && windows[1].display==0);
 displays=2;sdl_host_displays_changed();assert(!consoles[1].fullscreen);
 toggle_full_screen(&consoles[1]);assert(consoles[1].fullscreen && windows[1].display==1);
 /* A destroyed secondary cannot reserve a monitor. */
 consoles[2].real_window=NULL;toggle_full_screen(&consoles[0]);assert(consoles[0].fullscreen);
 assert(!sdl_set_fullscreen(&consoles[2],true));
 assert(positions && changes);
 /* GTK connectors differ from SDL descriptions; GTK order differs from SDL. */
 memset(consoles,0,sizeof(consoles));memset(windows,0,sizeof(windows));
 displays=2;names[0]="Headless output 1";names[1]="Headless output 3";
 bounds[0]=(SDL_Rect){0,0,1920,1080};bounds[1]=(SDL_Rect){1920,0,1600,900};
 setenv("QEMU_SDL_OUTPUT_0","HEADLESS-2",1);
 setenv("QEMU_SDL_OUTPUT_BOUNDS_0","1920,0,1600,900",1);
 setenv("QEMU_SDL_OUTPUT_1","HEADLESS-1",1);
 setenv("QEMU_SDL_OUTPUT_BOUNDS_1","0,0,1920,1080",1);
 setenv("QEMU_SDL_OUTPUT_FULLSCREEN_1","true",1);
 for(int i=0;i<2;i++) {consoles[i].idx=i;consoles[i].real_window=&windows[i];sdl_configure_output(&consoles[i]);toggle_full_screen(&consoles[i]);}
 assert(consoles[0].fullscreen && windows[0].display==1);
 assert(consoles[1].fullscreen && windows[1].display==0);
 /* Connector name has priority over stale geometry. */
 names[0]="HEADLESS-2";assert(sdl_target_display(&consoles[0])==0);
 names[0]="Headless output 1";
 /* Empty names still resolve by logical bounds, with independent toggles. */
 names[0]="";names[1]="";
 assert(sdl_target_display(&consoles[0])==1 && sdl_target_display(&consoles[1])==0);
 toggle_full_screen(&consoles[0]);assert(!consoles[0].fullscreen && consoles[1].fullscreen);
 toggle_full_screen(&consoles[0]);assert(consoles[0].fullscreen);
 /* Reordering preserves bounds identity; unplug exits only the missing output. */
 SDL_Rect swap=bounds[0];bounds[0]=bounds[1];bounds[1]=swap;
 sdl_host_displays_changed();assert(windows[0].display==0 && windows[1].display==1);
 displays=1;sdl_host_displays_changed();assert(consoles[0].fullscreen && !consoles[1].fullscreen);
 displays=2;sdl_host_displays_changed();assert(!consoles[1].fullscreen);
 /* SDL-only Automatic pins bounds, never an empty/duplicate name. */
 memset(consoles,0,sizeof(consoles));
 for(int i=0;i<2;i++) {
  consoles[i].idx=i;consoles[i].real_window=&windows[i];
  int d=sdl_target_display(&consoles[i]);sdl_pin_output(&consoles[i],d);
  assert(consoles[i].target_monitor==NULL && consoles[i].has_target_bounds);
  assert(sdl_set_fullscreen(&consoles[i],true));
 }
 assert(windows[0].display==0 && windows[1].display==1);
 swap=bounds[0];bounds[0]=bounds[1];bounds[1]=swap;
 sdl_host_displays_changed();assert(windows[0].display==1 && windows[1].display==0);
 names[0]=names[1]="duplicate";
 struct sdl2_console duplicate={0};sdl_pin_output(&duplicate,1);
 assert(!duplicate.target_monitor && duplicate.has_target_bounds);
 /* A selected but missing bounds target must not fall back by index. */
 consoles[0].target_bounds.x=-999;assert(sdl_target_display(&consoles[0])==-1);

 return 0;
}
'''
    # Verify invalid environment fails rather than accepting separators or booleans.
    with tempfile.TemporaryDirectory(prefix='tryomarchy-sdl-displays-') as tmp:
        root = Path(tmp)
        flags = shlex.split(subprocess.check_output(['pkg-config', '--cflags', '--libs', 'glib-2.0'], text=True))
        (root / 'test.c').write_text(prefix + mock + state + functions + tests)
        subprocess.run(['cc', '-std=gnu11', '-O2', str(root / 'test.c'), '-o', str(root / 'test'), *flags], check=True)
        subprocess.run([str(root / 'test')], check=True)
        invalid = prefix + mock + state + functions + 'int main(void) {sdl_configure_output(&consoles[0]);return 0;}'
        (root / 'invalid.c').write_text(invalid)
        subprocess.run(['cc', '-std=gnu11', '-O2', str(root / 'invalid.c'), '-o', str(root / 'invalid'), *flags], check=True)
        for key, value in [('QEMU_SDL_OUTPUT_0','DP-1,DP-2'), ('QEMU_SDL_OUTPUT_FULLSCREEN_0','yes'), *[('QEMU_SDL_OUTPUT_BOUNDS_0', b) for b in ('', '0,0,0,900', '0,0,1600,-1', '0,0,1600,900x', '0,0,1600,900,1', '0,0,1600', '1048577,0,1600,900', '0,0,32769,900', '0,0,999999999999999999999,900', ' 0,0,1600,900')]]:
            env = os.environ.copy();env[key] = value
            assert subprocess.run([str(root / 'invalid')], env=env).returncode == 1
        if real_sdl:
            actual = prefix + '#include <SDL.h>\n' + state + functions + r'''
int main(void) {
 assert(SDL_Init(SDL_INIT_VIDEO)==0);
 assert(SDL_GetNumVideoDisplays()>0);
 for(int i=0;i<2;i++) {
  consoles[i].idx=i;consoles[i].target_monitor=g_strdup(SDL_GetDisplayName(0));
  consoles[i].real_window=SDL_CreateWindow("test",SDL_WINDOWPOS_UNDEFINED_DISPLAY(0),SDL_WINDOWPOS_UNDEFINED_DISPLAY(0),640,480,SDL_WINDOW_HIDDEN);
  assert(consoles[i].real_window);
 }
 assert(sdl_set_fullscreen(&consoles[0],true));
 assert(!sdl_set_fullscreen(&consoles[1],true));
 assert(sdl_set_fullscreen(&consoles[0],false));
 assert(sdl_set_fullscreen(&consoles[1],true));
 for(int i=0;i<2;i++) {SDL_DestroyWindow(consoles[i].real_window);g_free(consoles[i].target_monitor);}
 SDL_Quit();return 0;
}
'''
            (root / 'real.c').write_text(actual)
            flags += shlex.split(subprocess.check_output(['pkg-config', '--cflags', '--libs', 'sdl2'], text=True))
            subprocess.run(['cc', '-std=gnu11', '-O2', str(root / 'real.c'), '-o', str(root / 'real'), *flags], check=True)
            env = os.environ.copy();env['SDL_VIDEODRIVER']='dummy'
            subprocess.run([str(root / 'real')], env=env, check=True)
    print('ok - per-window fullscreen, connector/description and empty names, logical bounds, Automatic pinning, collisions, reorder, unplug/replug, validation' + (', actual SDL dummy' if real_sdl else ''))


def check_qemu(qemu):
    device = {"driver":"virtio-gpu-pci", "id":"gpu0", "max_outputs":3,
              "outputs":[{"name":f"Omarchy {i+1}","xres":1280+i*320,"yres":800+i*100} for i in range(3)]}
    env = os.environ.copy()
    env['SDL_VIDEODRIVER'] = 'dummy'
    env['QEMU_SDL_TITLE_FROM_NAME'] = '1'
    for i in range(3):
        env[f'QEMU_SDL_OUTPUT_{i}'] = ''
        env[f'QEMU_SDL_OUTPUT_FULLSCREEN_{i}'] = 'true'
    commands = [{"execute":"qmp_capabilities"},
                {"execute":"qom-get","arguments":{"path":"/machine/peripheral/gpu0","property":"max_outputs"},"id":"outputs"},
                {"execute":"quit"}]
    result = subprocess.run([str(qemu), '-nodefaults', '-machine', 'q35', '-accel', 'tcg',
        '-m', '128', '-S', '-name', 'Try Omarchy', '-vga', 'none', '-device', json.dumps(device),
        '-display', 'sdl,gl=off', '-qmp', 'stdio'], env=env,
        input=''.join(json.dumps(c)+'\n' for c in commands), capture_output=True, text=True, timeout=30)
    assert result.returncode == 0, result.stderr
    replies = [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]
    assert any(r.get('id')=='outputs' and r.get('return')==3 for r in replies), result.stdout
    print('ok - whole QEMU SDL dummy startup, three configured outputs, per-output sizes, QMP shutdown')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--manifest', type=Path)
    group.add_argument('--source', type=Path)
    parser.add_argument('--real-sdl', action='store_true')
    parser.add_argument('--qemu', type=Path, help='also smoke-test a built QEMU with SDL dummy')
    args = parser.parse_args()
    if args.manifest:
        focus.check = lambda source: check(source, args.real_sdl)
        focus.from_manifest(args.manifest)
    else:
        check(args.source.read_text(), args.real_sdl)

    if args.qemu:
        check_qemu(args.qemu)
