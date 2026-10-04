/* Headless test only: inject precise SDL events into the unmodified QEMU binary.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */
#include <SDL.h>
#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

static SDL_Window *window;

SDL_Window *SDL_CreateWindow(const char *title, int x, int y, int w, int h,
                             Uint32 flags)
{
    SDL_Window *(*create_window)(const char *, int, int, int, int, Uint32) =
        dlsym(RTLD_NEXT, "SDL_CreateWindow");

    if (!create_window) {
        abort();
    }
    window = create_window(title, x, y, w, h, flags);
    return window;
}

int SDL_PollEvent(SDL_Event *event)
{
    static int (*poll_event)(SDL_Event *);
    static FILE *input;
    float x, y;
    unsigned direction;
    const char *path = getenv("QEMU_SDL_SCROLL_EVENTS");

    if (!poll_event) {
        poll_event = dlsym(RTLD_NEXT, "SDL_PollEvent");
        if (!poll_event) {
            abort();
        }
    }
    if (!event || !path || !window) {
        return poll_event(event);
    }
    if (!input) {
        input = fopen(path, "r");
        if (input) {
            unlink(path);
        }
    }
    if (input) {
        if (fscanf(input, "%f %f %u", &x, &y, &direction) == 3) {
            SDL_zero(*event);
            event->type = SDL_MOUSEWHEEL;
            event->wheel.windowID = SDL_GetWindowID(window);
            event->wheel.x = (int)x;
            event->wheel.y = (int)y;
            event->wheel.preciseX = x;
            event->wheel.preciseY = y;
            event->wheel.direction = direction;
            fprintf(stderr, "injected SDL wheel: x=%g y=%g direction=%u\n",
                    x, y, direction);
            return 1;
        }
        fclose(input);
        input = NULL;
    }
    return poll_event(event);
}
