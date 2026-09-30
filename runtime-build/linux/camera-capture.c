// Capture from the camera portal's restricted PipeWire remote on descriptor 3.
// stdout is packed 1280x720 NV12, with no headers or camera data in diagnostics.
#include <gst/app/gstappsink.h>
#include <gst/video/video.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#define WIDTH 1280
#define HEIGHT 720

// Keep the provider running while its element captures. Both share the
// GStreamer PipeWire core for fd 3; opening two cores on one socket is invalid.
static GstElement *camera_source(GstDeviceProvider *provider, const char *wanted)
{
    if (!gst_device_provider_start(provider)) return NULL;
    gint64 deadline = g_get_monotonic_time() + 5 * G_TIME_SPAN_SECOND;
    GstElement *source = NULL;
    do {
        GList *devices = gst_device_provider_get_devices(provider);
        for (GList *item = devices; item && !source; item = item->next) {
            GstDevice *device = item->data;
            GstStructure *props = gst_device_get_properties(device);
            const char *class = props ? gst_structure_get_string(props, "media.class") : NULL;
            const char *name = props ? gst_structure_get_string(props, "node.name") : NULL;
            if (class && !strcmp(class, "Video/Source") && name &&
                (!*wanted || !strcmp(name, wanted))) {
                source = gst_device_create_element(device, "source");
                if (source) g_object_set(source, "target-object", name, NULL);
            }
            if (props) gst_structure_free(props);
        }
        g_list_free_full(devices, (GDestroyNotify)gst_object_unref);
        if (source) break;
        g_usleep(100000);
    } while (g_get_monotonic_time() < deadline);
    return source;
}

static void decoded(GstElement *decoder, GstPad *pad, gpointer convert)
{
    GstPad *sink = gst_element_get_static_pad(convert, "sink");
    if (!gst_pad_is_linked(sink)) gst_pad_link(pad, sink);
    gst_object_unref(sink);
}

// GstVideoFrame provides each plane's actual offset and stride. A camera or
// converter may pad rows; copying the whole buffer would corrupt the wire frame.
static bool write_frame(GstSample *sample)
{
    GstVideoInfo info;
    GstVideoFrame frame;
    if (!gst_video_info_from_caps(&info, gst_sample_get_caps(sample)) ||
        GST_VIDEO_INFO_FORMAT(&info) != GST_VIDEO_FORMAT_NV12 ||
        GST_VIDEO_INFO_WIDTH(&info) != WIDTH || GST_VIDEO_INFO_HEIGHT(&info) != HEIGHT ||
        !gst_video_frame_map(&frame, &info, gst_sample_get_buffer(sample), GST_MAP_READ))
        return false;
    bool success = true;
    for (unsigned plane = 0; plane < 2 && success; plane++) {
        const guint8 *base = GST_VIDEO_FRAME_PLANE_DATA(&frame, plane);
        int stride = GST_VIDEO_FRAME_PLANE_STRIDE(&frame, plane);
        if (ABS(stride) < WIDTH) { success = false; break; }
        for (unsigned row = 0; row < (plane ? HEIGHT/2 : HEIGHT); row++) {
            if (fwrite(base + (ptrdiff_t)row * stride, 1, WIDTH, stdout) != WIDTH) {
                success = false; break;
            }
        }
    }
    gst_video_frame_unmap(&frame);
    return success && fflush(stdout) == 0;
}

int main(int argc, char **argv)
{
    gst_init(NULL, NULL);
    const char *wanted = argc > 1 ? argv[1] : "";
    GstDeviceProvider *provider = gst_device_provider_factory_get_by_name("pipewiredeviceprovider");
    if (!provider) { fprintf(stderr, "Camera device discovery is unavailable\n"); return 1; }
    g_object_set(provider, "fd", 3, "client-name", "Try Omarchy Camera", NULL);
    GstElement *source = camera_source(provider, wanted);
    if (!source) {
        fprintf(stderr, "%s\n", *wanted ? "The selected camera is unavailable" :
                "No camera is available through your desktop camera service");
        return 1;
    }
    GstElement *pipeline = gst_pipeline_new("camera");
    GstElement *decoder = gst_element_factory_make("decodebin", NULL);
    GstElement *convert = gst_element_factory_make("videoconvert", NULL);
    GstElement *scale = gst_element_factory_make("videoscale", NULL);
    GstElement *rate = gst_element_factory_make("videorate", NULL);
    GstElement *sink = gst_element_factory_make("appsink", NULL);
    if (!pipeline || !source || !decoder || !convert || !scale || !rate || !sink) {
        fprintf(stderr, "Camera capture components are missing\n");
        return 1;
    }
    g_object_set(source, "client-name", "Try Omarchy Camera", "always-copy", TRUE, NULL);
    GstStructure *properties = gst_structure_new("props", "media.type", G_TYPE_STRING, "Video",
        "media.category", G_TYPE_STRING, "Capture", "media.role", G_TYPE_STRING, "Camera",
        "node.dont-fallback", G_TYPE_BOOLEAN, TRUE, NULL);
    g_object_set(source, "stream-properties", properties, NULL);
    gst_structure_free(properties);
    GstCaps *caps = gst_caps_new_simple("video/x-raw", "format", G_TYPE_STRING, "NV12",
        "width", G_TYPE_INT, WIDTH, "height", G_TYPE_INT, HEIGHT,
        "framerate", GST_TYPE_FRACTION, 30, 1, NULL);
    g_object_set(sink, "caps", caps, "sync", FALSE, "max-buffers", 2u, "drop", TRUE, NULL);
    gst_caps_unref(caps);
    gst_bin_add_many(GST_BIN(pipeline), source, decoder, convert, scale, rate, sink, NULL);
    g_signal_connect(decoder, "pad-added", G_CALLBACK(decoded), convert);
    if (!gst_element_link(source, decoder) ||
        !gst_element_link_many(convert, scale, rate, sink, NULL)) {
        fprintf(stderr, "Could not link the camera capture components\n");
        gst_object_unref(pipeline);
        return 1;
    }
    GstBus *bus = gst_element_get_bus(pipeline);
    bool success = gst_element_set_state(pipeline, GST_STATE_PLAYING) != GST_STATE_CHANGE_FAILURE;
    gint64 last_frame = g_get_monotonic_time();
    for (;;) {
        GstSample *sample = gst_app_sink_try_pull_sample(GST_APP_SINK(sink), GST_SECOND);
        if (sample) {
            last_frame = g_get_monotonic_time();
            success = write_frame(sample);
            gst_sample_unref(sample);
            if (!success) break;
        } else {
            GstMessage *message = gst_bus_pop_filtered(bus, GST_MESSAGE_ERROR | GST_MESSAGE_EOS);
            if (message) {
                if (GST_MESSAGE_TYPE(message) == GST_MESSAGE_ERROR) {
                    GError *error = NULL;
                    gst_message_parse_error(message, &error, NULL);
                    fprintf(stderr, "Camera pipeline: %.512s\n", error->message);
                    g_error_free(error);
                    success = false;
                }
                gst_message_unref(message);
                break;
            }
            if (gst_app_sink_is_eos(GST_APP_SINK(sink))) break;
            if (!success) { fprintf(stderr, "Could not start the camera pipeline\n"); break; }
            if (g_get_monotonic_time() - last_frame > 10 * G_TIME_SPAN_SECOND) {
                fprintf(stderr, "The camera stopped providing frames\n");
                success = false;
                break;
            }
        }
    }
    gst_element_set_state(pipeline, GST_STATE_NULL);
    gst_object_unref(bus);
    gst_object_unref(pipeline);
    gst_device_provider_stop(provider);
    gst_object_unref(provider);
    close(3);
    return success ? 0 : 1;
}
