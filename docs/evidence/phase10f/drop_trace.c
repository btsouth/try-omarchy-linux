/* Private diagnostic shim for SDL2 events in the disposable GNOME VM. */
#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdint.h>
#include <stdio.h>

int SDL_PollEvent(void *event)
{
    static int (*next)(void *);
    static unsigned seen;
    if (!next) {
        next = dlsym(RTLD_NEXT, "SDL_PollEvent");
    }
    int result = next(event);
    if (result && event) {
        uint32_t type = *(uint32_t *)event;
        if (seen++ < 12) {
            fprintf(stderr, "PHASE10F SDL observed event type=0x%x\n", type);
            fflush(stderr);
        }
        if (type >= 0x1000 && type <= 0x1003) {
            fprintf(stderr, "PHASE10F SDL drop event type=0x%x\n", type);
            fflush(stderr);
        }
    }
    return result;
}
