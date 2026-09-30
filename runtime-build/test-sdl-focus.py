#!/usr/bin/env python3
"""Check Linux input ownership using the patched QEMU SDL handlers."""
import argparse
import hashlib
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import urllib.request


def function(source, name):
    # Locate the definition by its line, then balance its body braces.
    start = source.rfind('\nstatic ', 0, source.index(name + '(')) + 1
    opening = source.index('{', start)
    depth = 1
    end = opening + 1
    while depth:
        depth += (source[end] == '{') - (source[end] == '}')
        end += 1
    return source[start:end] + '\n'


def check(source):
    harness = r'''
#include <assert.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
typedef struct { int flags, x, y, w, h; } SDL_Window;
typedef int QemuConsole;
typedef struct {int width,height,width_mm,height_mm;} QemuUIInfo;
typedef struct {struct {uint32_t windowID;int event;} window;} SDL_Event;
struct options {bool has_window_close,window_close;};
struct sdl2_console {SDL_Window *real_window;struct {QemuConsole *con;} dcl;
    struct options *opts;bool hidden,ignore_hotkeys;};
static SDL_Window window={1,100,100,640,480}, other;
static struct options opts;
static QemuConsole con;
static struct sdl2_console console={&window,{&con},&opts,false,false};
static SDL_Window *mouse_focus=&window;
static const char *driver="wayland";
static bool focus_keyboard_grab=true,keyboard_released,absolute=true;
static int gui_grab,gui_fullscreen,gui_saved_grab,absolute_enabled,guest_cursor,guest_x,guest_y;
static int keyboard,pointer,cursor,guest_sprite,shutdown_action;
static bool allow_close;
static int global_x=110,global_y=110,local_x=10,local_y=10;
enum {SDL_FALSE,SDL_TRUE,SDL_WINDOW_INPUT_FOCUS=1,SDL_WINDOW_FULLSCREEN_DESKTOP=2,
 SDL_WINDOWEVENT_RESIZED=10,SDL_WINDOWEVENT_SIZE_CHANGED,SDL_WINDOWEVENT_DISPLAY_CHANGED,
 SDL_WINDOWEVENT_EXPOSED,SDL_WINDOWEVENT_FOCUS_GAINED,SDL_WINDOWEVENT_ENTER,
 SDL_WINDOWEVENT_LEAVE,SDL_WINDOWEVENT_FOCUS_LOST,SDL_WINDOWEVENT_RESTORED,
 SDL_WINDOWEVENT_MINIMIZED,SDL_WINDOWEVENT_CLOSE,SDL_WINDOWEVENT_SHOWN,SDL_WINDOWEVENT_HIDDEN,
 GUI_REFRESH_INTERVAL_DEFAULT=30,SHUTDOWN_ACTION_POWEROFF,SHUTDOWN_CAUSE_HOST_UI};
#define SDL_VERSION_ATLEAST(a,b,c) 1
#define MAX(a,b) ((a)>(b)?(a):(b))
#define g_strcmp0 strcmp
#define g_getenv getenv
#define g_ascii_strtod strtod
static const char *SDL_GetCurrentVideoDriver(void) {return driver;}
static SDL_Window *SDL_GetMouseFocus(void) {return mouse_focus;}
static int SDL_GetWindowFlags(SDL_Window *w) {return w->flags;}
static void SDL_GetWindowPosition(SDL_Window *w,int *x,int *y) {*x=w->x;*y=w->y;}
static void SDL_GetWindowSize(SDL_Window *w,int *x,int *y) {*x=w->w;*y=w->h;}
#define SDL_GetWindowSizeInPixels SDL_GetWindowSize
static int SDL_GetGlobalMouseState(int *x,int *y) {*x=global_x;*y=global_y;return 0;}
static int SDL_GetMouseState(int *x,int *y) {*x=local_x;*y=local_y;return 0;}
static void SDL_SetWindowKeyboardGrab(SDL_Window *w,int v) {keyboard=v;}
static void SDL_SetWindowGrab(SDL_Window *w,int v) {pointer=keyboard=v;}
static void SDL_SetWindowMouseGrab(SDL_Window *w,int v) {pointer=v;}
static void SDL_SetWindowFullscreen(SDL_Window *w,int v) {}
static void SDL_SetCursor(int c) {}
static void SDL_WarpMouseInWindow(SDL_Window *w,int x,int y) {}
static void SDL_HideWindow(SDL_Window *w) {}
static bool qemu_console_is_graphic(QemuConsole *c) {return true;}
static bool qemu_input_is_absolute(QemuConsole *c) {return absolute;}
static void sdl_hide_cursor(struct sdl2_console *s) {cursor=0;}
static void sdl_show_cursor(struct sdl2_console *s) {cursor=1;}
static void sdl_update_caption(struct sdl2_console *s) {}
static struct sdl2_console *get_scon_from_window(uint32_t id) {return id==1?&console:NULL;}
static void sdl2_redraw(struct sdl2_console *s) {}
static int get_mod_state(void) {return 0;}
static void qemu_console_set_ui_info(QemuConsole *c,QemuUIInfo *i,bool b) {}
static void qemu_console_listener_set_refresh(void *d,int n) {}
static void qemu_system_shutdown_request(int n) {}
static int qemu_console_get_index(QemuConsole *c) {return 0;}
static void qapi_event_send_display_close_request(int n) {}
''' + ''.join(function(source, name) for name in (
        'sdl_pointer_outside', 'sdl_focus_keyboard_grab', 'sdl_grab_start',
        'sdl_grab_end', 'absolute_mouse_grab', 'sdl_release_keyboard',
        'handle_windowevent', 'toggle_full_screen')) + r'''
static void event(int type) {SDL_Event e={.window={1,type}};handle_windowevent(&e);}
int main(void) {
    /* Absolute input captures shortcuts without confining the host pointer. */
    event(SDL_WINDOWEVENT_ENTER);assert(keyboard && gui_grab && !pointer && !cursor);
    mouse_focus=&other;event(SDL_WINDOWEVENT_LEAVE);
    assert(!keyboard && !gui_grab && !pointer && cursor);
    /* Host panels need not change keyboard focus. Stale positions cannot regrab. */
    assert(window.flags & SDL_WINDOW_INPUT_FOCUS);
    event(SDL_WINDOWEVENT_FOCUS_GAINED);assert(!keyboard && !gui_grab);
    sdl_grab_start(&console);assert(!keyboard && !gui_grab);
    mouse_focus=&window;event(SDL_WINDOWEVENT_ENTER);assert(keyboard && gui_grab && !pointer);
    /* The release hotkey survives pointer leave/reentry until an explicit recapture. */
    sdl_release_keyboard(&console,true);assert(!keyboard && !gui_grab && cursor);
    event(SDL_WINDOWEVENT_ENTER);assert(!keyboard && !gui_grab);
    sdl_release_keyboard(&console,false);assert(keyboard && gui_grab && !pointer);
    window.flags=0;event(SDL_WINDOWEVENT_FOCUS_LOST);assert(!keyboard && !gui_grab);
    event(SDL_WINDOWEVENT_ENTER);assert(!keyboard && !gui_grab);
    window.flags=SDL_WINDOW_INPUT_FOCUS;event(SDL_WINDOWEVENT_FOCUS_GAINED);
    assert(keyboard && gui_grab);
    /* Preserve the X11 safeguard when SDL's pointer focus is stale. */
    driver="x11";global_x=0;event(SDL_WINDOWEVENT_LEAVE);assert(!keyboard && !gui_grab);
    event(SDL_WINDOWEVENT_FOCUS_GAINED);assert(!keyboard && !gui_grab);
    global_x=110;event(SDL_WINDOWEVENT_ENTER);assert(keyboard && gui_grab);
    sdl_grab_end(&console);driver="wayland";
    /* Relative and fullscreen input still need a pointer grab. */
    absolute=false;sdl_grab_start(&console);assert(pointer && keyboard);
    sdl_release_keyboard(&console,true);assert(!pointer && !keyboard);
    keyboard_released=false;absolute=true;gui_fullscreen=true;
    sdl_grab_start(&console);assert(pointer && keyboard);
    sdl_release_keyboard(&console,true);assert(!pointer && !keyboard);
    gui_fullscreen=false;keyboard_released=false;sdl_grab_start(&console);
    toggle_full_screen(&console);assert(gui_fullscreen && pointer && keyboard);
    toggle_full_screen(&console);assert(!gui_fullscreen && !pointer && keyboard);
    /* Do not alter runtimes which did not opt into focus keyboard capture. */
    focus_keyboard_grab=false;keyboard_released=false;gui_fullscreen=false;
    mouse_focus=&other;sdl_grab_start(&console);assert(pointer && keyboard);
    return 0;
}
'''
    with tempfile.TemporaryDirectory(prefix='tryomarchy-sdl-focus-') as tmp:
        root = Path(tmp)
        (root / 'test.c').write_text(harness)
        subprocess.run(['cc', '-std=gnu11', '-O2', str(root / 'test.c'),
                        '-o', str(root / 'test')], check=True)
        subprocess.run([str(root / 'test')], check=True)
    print('ok - Wayland/X11 leave, stale focus, recapture, hotkey release, relative/fullscreen grabs')


def from_manifest(manifest):
    # Test exactly the archive and ordered patch list used by the Linux build.
    module = manifest.read_text().split('  - name: qemu\n', 1)[1].split('\n  - name:', 1)[0]
    url = re.search(r'        url: (\S+)', module)[1]
    digest = re.search(r'        sha256: (\w+)', module)[1]
    patches = re.findall(r'      - type: patch\n        path: (\S+)', module)
    with tempfile.TemporaryDirectory(prefix='tryomarchy-qemu-focus-') as tmp:
        root = Path(tmp)
        archive = root / 'qemu.tar.xz'
        urllib.request.urlretrieve(url, archive)
        assert hashlib.sha256(archive.read_bytes()).hexdigest() == digest, 'QEMU archive checksum'
        with tarfile.open(archive) as tar:
            tar.extractall(root, members=(m for m in tar if m.isfile()), filter='data')
        source = next(root.glob('qemu-*/ui/sdl2.c'))
        for patch in patches:
            subprocess.run(['patch', '-p1', '--batch', '--forward', '-i',
                            str((manifest.parent / patch).resolve())],
                           cwd=source.parents[1], check=True, stdout=subprocess.DEVNULL)
        check(source.read_text())


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    group = parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--manifest', type=Path)
    group.add_argument('--source', type=Path)
    args = parser.parse_args()
    if args.manifest:
        from_manifest(args.manifest)
    else:
        check(args.source.read_text())
