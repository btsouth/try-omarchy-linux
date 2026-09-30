// No camera, PipeWire server or display is used by this frame-packing test.
#define main camera_helper_main
#include "camera-capture.c"
#undef main
#include <assert.h>

static void check_packing(bool negative)
{
    int stride = WIDTH + 32;
    size_t uv_start = stride * HEIGHT + 64;
    size_t size = uv_start + stride * (HEIGHT/2);
    GstBuffer *buffer = gst_buffer_new_allocate(NULL, size, NULL);
    GstMapInfo map;
    assert(gst_buffer_map(buffer, &map, GST_MAP_WRITE));
    memset(map.data, 0xee, size);
    gsize offsets[GST_VIDEO_MAX_PLANES] = {0};
    gint strides[GST_VIDEO_MAX_PLANES] = {0};
    for (unsigned plane = 0; plane < 2; plane++) {
        unsigned rows = plane ? HEIGHT/2 : HEIGHT;
        offsets[plane] = (plane ? uv_start : 0) + (negative ? (rows-1)*stride : 0);
        strides[plane] = negative ? -stride : stride;
        for (unsigned row = 0; row < rows; row++) {
            unsigned char value = plane ? 128 + row % 64 : 16 + row % 208;
            memset(map.data + offsets[plane] + (ptrdiff_t)row * strides[plane], value, WIDTH);
        }
    }
    gst_buffer_unmap(buffer, &map);
    gst_buffer_add_video_meta_full(buffer, GST_VIDEO_FRAME_FLAG_NONE, GST_VIDEO_FORMAT_NV12,
                                  WIDTH, HEIGHT, 2, offsets, strides);
    GstCaps *caps = gst_caps_new_simple("video/x-raw", "format", G_TYPE_STRING, "NV12",
        "width", G_TYPE_INT, WIDTH, "height", G_TYPE_INT, HEIGHT, NULL);
    GstSample *sample = gst_sample_new(buffer, caps, NULL, NULL);
    FILE *output = tmpfile();
    assert(output);
    int original = dup(STDOUT_FILENO);
    assert(dup2(fileno(output), STDOUT_FILENO) >= 0);
    assert(write_frame(sample));
    assert(ftell(output) == WIDTH * HEIGHT * 3 / 2);
    rewind(output);
    for (unsigned plane = 0; plane < 2; plane++)
        for (unsigned row = 0; row < (plane ? HEIGHT/2 : HEIGHT); row++)
            for (unsigned column = 0; column < WIDTH; column++)
                assert(fgetc(output) == (int)(plane ? 128 + row % 64 : 16 + row % 208));
    assert(fgetc(output) == EOF);
    assert(dup2(original, STDOUT_FILENO) >= 0);
    close(original);
    fclose(output);
    gst_sample_unref(sample);
    gst_caps_unref(caps);
    gst_buffer_unref(buffer);
}

int main(void)
{
    gst_init(NULL, NULL);
    check_packing(false);
    check_packing(true);
    fprintf(stderr, "NV12 padded and negative strides passed\n");
    return 0;
}
