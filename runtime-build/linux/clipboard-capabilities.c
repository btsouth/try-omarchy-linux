/* Detect data-control without creating a surface or reading the clipboard. */
#include <stdio.h>
#include <string.h>
#include <wayland-client.h>

static int supported;
static void global(void *data, struct wl_registry *registry, uint32_t name,
                   const char *interface, uint32_t version)
{
    if (!strcmp(interface, "zwlr_data_control_manager_v1") ||
        !strcmp(interface, "ext_data_control_manager_v1"))
        supported = 1;
}
static void removed(void *data, struct wl_registry *registry, uint32_t name) {}
static const struct wl_registry_listener listener = { global, removed };
int main(void)
{
    struct wl_display *display = wl_display_connect(NULL);
    if (!display) return 1;
    struct wl_registry *registry = wl_display_get_registry(display);
    wl_registry_add_listener(registry, &listener, NULL);
    int result = wl_display_roundtrip(display);
    wl_registry_destroy(registry);
    wl_display_disconnect(display);
    if (result < 0 || !supported) return 1;
    puts("data-control");
    return 0;
}
