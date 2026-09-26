I hit this with Venus on an RTX 4070 SUPER (driver 615.71.09, QEMU 11.1.1 with the SDL display, virglrenderer 1.3.0, guest Mesa 26.2.3) and tracked it down to the NVIDIA GL side of the path vrend uses for the prime buffer.

The prime blit buffer arrives as a LINEAR XRGB8888 dma-buf. `vrend_renderer_pipe_resource_set_type` imports it with `virgl_egl_image_from_dmabuf`, which passes the correct pitch, and `vrend_resource_alloc_texture` then binds the EGLImage with `glEGLImageTargetTexStorageEXT(GL_TEXTURE_2D)` because texture storage is available. NVIDIA reports LINEAR XRGB8888 as `external_only` in `eglQueryDmaBufModifiersEXT`. On the same image, `glEGLImageTargetTexture2DOES(GL_TEXTURE_2D)` fails with `GL_INVALID_OPERATION` as it should, but `glEGLImageTargetTexStorageEXT` succeeds and the texture samples rows at `align(width * 4, 32)` no matter what pitch the image was created with. Sampling the same EGLImage through `GL_TEXTURE_EXTERNAL_OES` honors the pitch.

That matches everything above: frames are only right when `align(width * 4, 256)` happens to equal `align(width * 4, 32)`, i.e. when the width is a multiple of 64. So raising `WSI_PRIME_LINEAR_STRIDE_ALIGN` to 1024 won't help, it only changes which widths happen to work. 32 does, since that is exactly the pitch NVIDIA assumes. With the prime stride alignment set to 32 for NVIDIA renderers in `vn_wsi_init` (patch attached), vkgears floated at 997x611 and 1001x700 and tiled at 1872x982 renders cleanly in the guest. Stock Mesa shears at 997x611 and tiled. A fully tight pitch is not an option either: for a 1001 px wide image NVIDIA accepts a 4004 byte pitch and then reads it at 4000, even through `GL_TEXTURE_EXTERNAL_OES`.

Standalone repro, no QEMU involved (attached `nv-dmabuf-pitch.c`). It copies a coordinate pattern into a dma-buf exportable VkBuffer at a chosen pitch, imports it with EGL, binds it one of three ways and reads it back. Build with `cc -O1 -o nv-dmabuf-pitch nv-dmabuf-pitch.c -lvulkan -lEGL -lGLESv2`. Results are the same in a GL 4.6 core and a GLES 3.2 context, with the modifier implicit or explicit LINEAR, and identical across repeated runs:

| Image | Pitch | TexStorage, 2D | TargetTexture2D, 2D | TargetTexture2D, external |
|---|---|---|---|---|
| 1896x1032 | 7680 | wrong, reads 7584 | GL_INVALID_OPERATION | correct |
| 1896x1032 | 7584 | correct | GL_INVALID_OPERATION | correct |
| 1001x700 | 4096 | wrong, reads 4032 | GL_INVALID_OPERATION | correct |
| 1001x700 | 4032 | correct | GL_INVALID_OPERATION | correct |
| 1001x700 | 4004 | wrong, reads 4032 | GL_INVALID_OPERATION | wrong, reads 4000 |

(External samplers aren't advertised in the desktop GL context, so that column is GLES only.)

This is an NVIDIA bug and I've added these details to the forum thread that is already open for it: https://forums.developer.nvidia.com/t/egl-import-via-egl-ext-image-dma-buf-import-modifiers-ignores-explicit-stride-causes-image-distortion-in-virtio-gpu-venus/364360

Until it's fixed, vrend can't avoid it on its side without sampling LINEAR imports as external textures, so a 32 byte prime stride on NVIDIA in Venus looks like the practical workaround. One case I haven't been able to test is a hybrid laptop where Venus renders on the NVIDIA dGPU but QEMU's GL context is on an Intel or AMD iGPU. There the importer is Mesa and it would get 32 byte aligned pitches instead of 256, and I don't know whether radeonsi accepts that. Keying on the renderer is the part of the patch I'm least sure about.
