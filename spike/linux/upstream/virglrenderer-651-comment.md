Still reproduces on NVIDIA 615.71.09 (RTX 4070 SUPER). One thing that may explain why !1605 made no difference on NVIDIA: on that driver vrend never reaches the line it changes. NVIDIA's desktop GL has texture storage and advertises `GL_EXT_EGL_image_storage`, so `vrend_resource_alloc_texture` takes the `glEGLImageTargetTexStorageEXT(GL_TEXTURE_2D)` branch, and that is the call that ignores the pitch.

In a standalone repro (attached, no QEMU: a dma-buf exported VkBuffer from the same GPU, imported with `eglCreateImageKHR` and read back), on a LINEAR XRGB8888 image:

- `glEGLImageTargetTexStorageEXT(GL_TEXTURE_2D)` succeeds and samples rows at `align(width * 4, 32)`
- `glEGLImageTargetTexture2DOES(GL_TEXTURE_2D)` fails with `GL_INVALID_OPERATION`, since NVIDIA reports LINEAR as `external_only`
- `glEGLImageTargetTexture2DOES(GL_TEXTURE_EXTERNAL_OES)` honors the pitch, as long as it's a multiple of 32 (a 4004 byte pitch for a 1001 px image is read wrong there too)

So an external texture path in vrend should work on NVIDIA, but it would have to skip the TexStorage branch for these imports and sample them with `samplerExternalOES`, which is more than !1605 does. Until then I'm carrying the 32 byte prime stride on the Venus side. The same details are on the NVIDIA thread.
