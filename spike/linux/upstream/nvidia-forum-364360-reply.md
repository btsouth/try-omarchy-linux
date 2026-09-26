Still happening on 615.71.09 (RTX 4070 SUPER, Linux 7.2.5). I narrowed it down to one entry point, in case it helps whoever picks this up.

For a LINEAR `DRM_FORMAT_XRGB8888` dma-buf imported with `eglCreateImageKHR(EGL_LINUX_DMA_BUF_EXT)` and an explicit `EGL_DMA_BUF_PLANE0_PITCH_EXT`:

- `eglQueryDmaBufModifiersEXT` reports LINEAR as `external_only`.
- `glEGLImageTargetTexture2DOES(GL_TEXTURE_2D, image)` fails with `GL_INVALID_OPERATION`, which matches that.
- `glEGLImageTargetTexStorageEXT(GL_TEXTURE_2D, image, NULL)` succeeds, but the texture samples rows at `align(width * 4, 32)` and ignores the pitch. Per EXT_EGL_image_storage the texture becomes a sibling of the EGLImage, so this call should either fail with `GL_INVALID_OPERATION` or honor the pitch.
- `glEGLImageTargetTexture2DOES(GL_TEXTURE_EXTERNAL_OES, image)` honors the pitch.

There's a second, smaller one. A pitch that isn't a multiple of 32 (4004 for a 1001 px wide image) is accepted by `eglCreateImageKHR` and then read wrong even through `GL_TEXTURE_EXTERNAL_OES`, at 4000. EGL_EXT_image_dma_buf_import says an unsupported pitch should fail with `EGL_BAD_ACCESS`.

Results are the same in a GL 4.6 core and a GLES 3.2 context, with the modifier implicit or explicit LINEAR, and identical across runs. The buffer in the repro is a dma-buf exported VkBuffer from the same GPU, so no other driver is involved. Repro attached (`nv-dmabuf-pitch.c`), built with `cc -O1 -o nv-dmabuf-pitch nv-dmabuf-pitch.c -lvulkan -lEGL -lGLESv2`:

```
./nv-dmabuf-pitch vulkan 1896 1032 7680 gl storage   # wrong, reads pitch 7584
./nv-dmabuf-pitch vulkan 1896 1032 7584 gl storage   # correct
./nv-dmabuf-pitch vulkan 1896 1032 7680 es target    # GL_INVALID_OPERATION
./nv-dmabuf-pitch vulkan 1896 1032 7680 es external  # correct
./nv-dmabuf-pitch vulkan 1001 700 4004 es external   # wrong, reads pitch 4000
```

virglrenderer binds the prime blit buffers from Venus with `glEGLImageTargetTexStorageEXT`, which is how this turns into the distorted frames in the first post. The virglrenderer side is tracked at https://gitlab.freedesktop.org/virgl/virglrenderer/-/issues/651
