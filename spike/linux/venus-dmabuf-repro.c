// Host-only replay of how a Venus frame reaches the guest compositor, without
// QEMU, to see which step corrupts it on a given host driver.
//
// On NVIDIA hosts Venus presents through Mesa's prime blit: the frame is
// copied into a linear VkBuffer whose row pitch is align(width * cpp, 256),
// and that buffer's memory is shared as a dma-buf with DRM_FORMAT_MOD_INVALID.
// virglrenderer imports it into the host EGL (virgl_egl_image_from_dmabuf:
// pitch, no modifier attributes) and binds the EGLImage to a GL_TEXTURE_2D
// with glEGLImageTargetTexStorageEXT (vrend_resource_alloc_texture).
//
//   venus-dmabuf-repro WIDTH HEIGHT buffer|image implicit|linear storage|target|external [PITCH]
//
//   buffer    linear VkBuffer, pitch align(width * 4, 256): the prime blit path
//   image     VK_IMAGE_TILING_LINEAR image at the driver's row pitch
//   implicit  no modifier attributes, as virglrenderer sends for MOD_INVALID
//   linear    explicit DRM_FORMAT_MOD_LINEAR
//   storage   glEGLImageTargetTexStorageEXT(GL_TEXTURE_2D), what vrend uses
//   target    glEGLImageTargetTexture2DOES(GL_TEXTURE_2D), vrend's fallback
//   external  glEGLImageTargetTexture2DOES(GL_TEXTURE_EXTERNAL_OES)
//   PITCH     buffer row pitch instead of Mesa's align(width * 4, 256)
//
// Each source pixel encodes its own coordinates, so a wrong readback shows
// which source pixel the importer fetched.
#define _GNU_SOURCE
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <vulkan/vulkan.h>
#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <GLES2/gl2.h>
#include <GLES2/gl2ext.h>
#include <libdrm/drm_fourcc.h>

#define VK(x) do { VkResult r_ = (x); if (r_ != VK_SUCCESS) { \
  fprintf(stderr, "%s failed: %d\n", #x, r_); exit(1); } } while (0)
#define DIE(...) do { fprintf(stderr, __VA_ARGS__); fputc('\n', stderr); exit(1); } while (0)
#define GLCHECK(what) do { GLenum e_ = glGetError(); if (e_) DIE("%s: GL error 0x%x", what, e_); } while (0)

typedef void (*PFNTEXSTORAGE)(GLenum target, GLeglImageOES image, const GLint *attribs);

static uint32_t pattern(uint32_t x, uint32_t y) {
  // XRGB8888 in memory: B, G, R, X. R = x low, G = y low, B = x and y high nibbles.
  uint32_t r = x & 0xff, g = y & 0xff, b = ((x >> 8) & 0xf) << 4 | ((y >> 8) & 0xf);
  return 0xffu << 24 | r << 16 | g << 8 | b;
}

static VkPhysicalDevice pd;
static VkDevice dev;
static VkQueue queue;

static int find_mem(uint32_t bits, VkMemoryPropertyFlags want) {
  VkPhysicalDeviceMemoryProperties mp;
  vkGetPhysicalDeviceMemoryProperties(pd, &mp);
  for (uint32_t i = 0; i < mp.memoryTypeCount; i++)
    if ((bits & (1u << i)) && (mp.memoryTypes[i].propertyFlags & want) == want) return (int)i;
  return -1;
}

static VkDeviceMemory alloc_export(VkMemoryRequirements mr, VkImage img, VkBuffer buf) {
  int mt = find_mem(mr.memoryTypeBits, VK_MEMORY_PROPERTY_DEVICE_LOCAL_BIT);
  if (mt < 0) mt = find_mem(mr.memoryTypeBits, 0);
  VkMemoryDedicatedAllocateInfo ded = { .sType = VK_STRUCTURE_TYPE_MEMORY_DEDICATED_ALLOCATE_INFO, .image = img, .buffer = buf };
  VkExportMemoryAllocateInfo exp = { .sType = VK_STRUCTURE_TYPE_EXPORT_MEMORY_ALLOCATE_INFO, .pNext = &ded, .handleTypes = VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT };
  VkMemoryAllocateInfo mai = { .sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO, .pNext = &exp, .allocationSize = mr.size, .memoryTypeIndex = (uint32_t)mt };
  VkDeviceMemory mem; VK(vkAllocateMemory(dev, &mai, NULL, &mem));
  return mem;
}

int main(int argc, char **argv) {
  if (argc != 6 && argc != 7) DIE("usage: %s WIDTH HEIGHT buffer|image implicit|linear storage|target|external [PITCH]", argv[0]);
  uint32_t W = atoi(argv[1]), H = atoi(argv[2]);
  int use_buffer = strcmp(argv[3], "buffer") == 0;
  int explicit_linear = strcmp(argv[4], "linear") == 0;
  const char *bind = argv[5];

  VkApplicationInfo app = { .sType = VK_STRUCTURE_TYPE_APPLICATION_INFO, .apiVersion = VK_API_VERSION_1_2 };
  VkInstanceCreateInfo ici = { .sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO, .pApplicationInfo = &app };
  VkInstance inst; VK(vkCreateInstance(&ici, NULL, &inst));
  uint32_t n = 8; VkPhysicalDevice pds[8]; VK(vkEnumeratePhysicalDevices(inst, &n, pds));
  VkPhysicalDeviceProperties props;
  for (uint32_t i = 0; i < n && !pd; i++) {
    vkGetPhysicalDeviceProperties(pds[i], &props);
    if (props.deviceType == VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU || props.deviceType == VK_PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU) pd = pds[i];
  }
  if (!pd) DIE("no GPU");
  printf("vulkan: %s\n", props.deviceName);

  float prio = 1;
  VkDeviceQueueCreateInfo qci = { .sType = VK_STRUCTURE_TYPE_DEVICE_QUEUE_CREATE_INFO, .queueFamilyIndex = 0, .queueCount = 1, .pQueuePriorities = &prio };
  const char *exts[] = { VK_KHR_EXTERNAL_MEMORY_FD_EXTENSION_NAME, VK_EXT_EXTERNAL_MEMORY_DMA_BUF_EXTENSION_NAME };
  VkDeviceCreateInfo dci = { .sType = VK_STRUCTURE_TYPE_DEVICE_CREATE_INFO, .queueCreateInfoCount = 1, .pQueueCreateInfos = &qci, .enabledExtensionCount = 2, .ppEnabledExtensionNames = exts };
  VK(vkCreateDevice(pd, &dci, NULL, &dev));
  vkGetDeviceQueue(dev, 0, 0, &queue);
  PFN_vkGetMemoryFdKHR getMemoryFd = (PFN_vkGetMemoryFdKHR)vkGetDeviceProcAddr(dev, "vkGetMemoryFdKHR");

  // Staging buffer holding the pattern, tightly packed.
  VkBufferCreateInfo sbci = { .sType = VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO, .size = (VkDeviceSize)W * H * 4, .usage = VK_BUFFER_USAGE_TRANSFER_SRC_BIT };
  VkBuffer staging; VK(vkCreateBuffer(dev, &sbci, NULL, &staging));
  VkMemoryRequirements smr; vkGetBufferMemoryRequirements(dev, staging, &smr);
  VkMemoryAllocateInfo smai = { .sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO, .allocationSize = smr.size,
    .memoryTypeIndex = (uint32_t)find_mem(smr.memoryTypeBits, VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT) };
  VkDeviceMemory smem; VK(vkAllocateMemory(dev, &smai, NULL, &smem));
  VK(vkBindBufferMemory(dev, staging, smem, 0));
  uint32_t *p; VK(vkMapMemory(dev, smem, 0, VK_WHOLE_SIZE, 0, (void **)&p));
  for (uint32_t y = 0; y < H; y++) for (uint32_t x = 0; x < W; x++) p[y * W + x] = pattern(x, y);
  vkUnmapMemory(dev, smem);

  VkCommandPoolCreateInfo cpci = { .sType = VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO, .queueFamilyIndex = 0 };
  VkCommandPool pool; VK(vkCreateCommandPool(dev, &cpci, NULL, &pool));
  VkCommandBufferAllocateInfo cbai = { .sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO, .commandPool = pool, .level = VK_COMMAND_BUFFER_LEVEL_PRIMARY, .commandBufferCount = 1 };
  VkCommandBuffer cb; VK(vkAllocateCommandBuffers(dev, &cbai, &cb));
  VkCommandBufferBeginInfo cbbi = { .sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO };
  VK(vkBeginCommandBuffer(cb, &cbbi));

  VkDeviceMemory mem; uint32_t pitch; uint64_t offset = 0;
  if (use_buffer) {
    // Mesa's prime blit destination: pitch align(width * cpp, 256), size aligned to 4096.
    pitch = argc == 7 ? (uint32_t)atoi(argv[6]) : (W * 4 + 255) & ~255u;
    VkExternalMemoryBufferCreateInfo emb = { .sType = VK_STRUCTURE_TYPE_EXTERNAL_MEMORY_BUFFER_CREATE_INFO, .handleTypes = VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT };
    VkBufferCreateInfo bci = { .sType = VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO, .pNext = &emb,
      .size = ((VkDeviceSize)pitch * H + 4095) & ~4095ull, .usage = VK_BUFFER_USAGE_TRANSFER_DST_BIT };
    VkBuffer buf; VK(vkCreateBuffer(dev, &bci, NULL, &buf));
    VkMemoryRequirements mr; vkGetBufferMemoryRequirements(dev, buf, &mr);
    mem = alloc_export(mr, VK_NULL_HANDLE, buf);
    VK(vkBindBufferMemory(dev, buf, mem, 0));
    for (uint32_t y = 0; y < H; y++) {
      VkBufferCopy region = { .srcOffset = (VkDeviceSize)y * W * 4, .dstOffset = (VkDeviceSize)y * pitch, .size = W * 4 };
      vkCmdCopyBuffer(cb, staging, buf, 1, &region);
    }
    printf("source: linear VkBuffer %ux%u pitch %u\n", W, H, pitch);
  } else {
    VkExternalMemoryImageCreateInfo emi = { .sType = VK_STRUCTURE_TYPE_EXTERNAL_MEMORY_IMAGE_CREATE_INFO, .handleTypes = VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT };
    VkImageCreateInfo imci = { .sType = VK_STRUCTURE_TYPE_IMAGE_CREATE_INFO, .pNext = &emi, .imageType = VK_IMAGE_TYPE_2D,
      .format = VK_FORMAT_B8G8R8A8_UNORM, .extent = { W, H, 1 }, .mipLevels = 1, .arrayLayers = 1, .samples = VK_SAMPLE_COUNT_1_BIT,
      .tiling = VK_IMAGE_TILING_LINEAR, .usage = VK_IMAGE_USAGE_TRANSFER_DST_BIT | VK_IMAGE_USAGE_SAMPLED_BIT,
      .initialLayout = VK_IMAGE_LAYOUT_UNDEFINED };
    VkImage img; VK(vkCreateImage(dev, &imci, NULL, &img));
    VkImageSubresource sub = { VK_IMAGE_ASPECT_COLOR_BIT, 0, 0 };
    VkSubresourceLayout lay; vkGetImageSubresourceLayout(dev, img, &sub, &lay);
    pitch = (uint32_t)lay.rowPitch; offset = lay.offset;
    VkMemoryRequirements mr; vkGetImageMemoryRequirements(dev, img, &mr);
    mem = alloc_export(mr, img, VK_NULL_HANDLE);
    VK(vkBindImageMemory(dev, img, mem, 0));
    VkImageMemoryBarrier b1 = { .sType = VK_STRUCTURE_TYPE_IMAGE_MEMORY_BARRIER, .dstAccessMask = VK_ACCESS_TRANSFER_WRITE_BIT,
      .oldLayout = VK_IMAGE_LAYOUT_UNDEFINED, .newLayout = VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL,
      .srcQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED, .dstQueueFamilyIndex = VK_QUEUE_FAMILY_IGNORED, .image = img,
      .subresourceRange = { VK_IMAGE_ASPECT_COLOR_BIT, 0, 1, 0, 1 } };
    vkCmdPipelineBarrier(cb, VK_PIPELINE_STAGE_TOP_OF_PIPE_BIT, VK_PIPELINE_STAGE_TRANSFER_BIT, 0, 0, NULL, 0, NULL, 1, &b1);
    VkBufferImageCopy reg = { .imageSubresource = { VK_IMAGE_ASPECT_COLOR_BIT, 0, 0, 1 }, .imageExtent = { W, H, 1 } };
    vkCmdCopyBufferToImage(cb, staging, img, VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL, 1, &reg);
    VkImageMemoryBarrier b2 = b1;
    b2.srcAccessMask = VK_ACCESS_TRANSFER_WRITE_BIT; b2.dstAccessMask = 0;
    b2.oldLayout = VK_IMAGE_LAYOUT_TRANSFER_DST_OPTIMAL; b2.newLayout = VK_IMAGE_LAYOUT_GENERAL;
    vkCmdPipelineBarrier(cb, VK_PIPELINE_STAGE_TRANSFER_BIT, VK_PIPELINE_STAGE_BOTTOM_OF_PIPE_BIT, 0, 0, NULL, 0, NULL, 1, &b2);
    printf("source: linear VkImage %ux%u pitch %u\n", W, H, pitch);
  }
  VK(vkEndCommandBuffer(cb));
  VkSubmitInfo si = { .sType = VK_STRUCTURE_TYPE_SUBMIT_INFO, .commandBufferCount = 1, .pCommandBuffers = &cb };
  VK(vkQueueSubmit(queue, 1, &si, VK_NULL_HANDLE));
  VK(vkQueueWaitIdle(queue));

  VkMemoryGetFdInfoKHR gfi = { .sType = VK_STRUCTURE_TYPE_MEMORY_GET_FD_INFO_KHR, .memory = mem, .handleType = VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT };
  int fd; VK(getMemoryFd(dev, &gfi, &fd));

  PFNEGLQUERYDEVICESEXTPROC queryDevices = (void *)eglGetProcAddress("eglQueryDevicesEXT");
  PFNEGLGETPLATFORMDISPLAYEXTPROC getPlatformDisplay = (void *)eglGetProcAddress("eglGetPlatformDisplayEXT");
  EGLDeviceEXT devs[8]; EGLint ndev = 0;
  if (!queryDevices || !queryDevices(8, devs, &ndev) || ndev == 0) DIE("no EGL devices");
  EGLDisplay dpy = getPlatformDisplay(EGL_PLATFORM_DEVICE_EXT, devs[0], NULL);
  if (!eglInitialize(dpy, NULL, NULL)) DIE("eglInitialize failed");
  eglBindAPI(EGL_OPENGL_ES_API);
  EGLint cattr[] = { EGL_CONTEXT_CLIENT_VERSION, 2, EGL_NONE };
  EGLContext ctx = eglCreateContext(dpy, EGL_NO_CONFIG_KHR, EGL_NO_CONTEXT, cattr);
  if (ctx == EGL_NO_CONTEXT || !eglMakeCurrent(dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, ctx)) DIE("no surfaceless GLES context");
  printf("gl: %s\n", glGetString(GL_RENDERER));

  PFNEGLQUERYDMABUFMODIFIERSEXTPROC queryMods = (void *)eglGetProcAddress("eglQueryDmaBufModifiersEXT");
  if (queryMods) {
    EGLuint64KHR mods[64]; EGLBoolean ext_only[64]; EGLint nm = 0;
    queryMods(dpy, DRM_FORMAT_XRGB8888, 64, mods, ext_only, &nm);
    for (EGLint i = 0; i < nm; i++)
      if (mods[i] == DRM_FORMAT_MOD_LINEAR) printf("egl: XRGB8888 LINEAR advertised, external_only=%d\n", ext_only[i]);
  }

  EGLint a[32]; int k = 0;
  a[k++] = EGL_WIDTH; a[k++] = W; a[k++] = EGL_HEIGHT; a[k++] = H;
  a[k++] = EGL_LINUX_DRM_FOURCC_EXT; a[k++] = DRM_FORMAT_XRGB8888;
  a[k++] = EGL_DMA_BUF_PLANE0_FD_EXT; a[k++] = fd;
  a[k++] = EGL_DMA_BUF_PLANE0_OFFSET_EXT; a[k++] = (EGLint)offset;
  a[k++] = EGL_DMA_BUF_PLANE0_PITCH_EXT; a[k++] = (EGLint)pitch;
  if (explicit_linear) {
    a[k++] = EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT; a[k++] = (EGLint)(DRM_FORMAT_MOD_LINEAR & 0xffffffff);
    a[k++] = EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT; a[k++] = (EGLint)(DRM_FORMAT_MOD_LINEAR >> 32);
  }
  a[k++] = EGL_NONE;
  PFNEGLCREATEIMAGEKHRPROC createImage = (void *)eglGetProcAddress("eglCreateImageKHR");
  EGLImageKHR eimg = createImage(dpy, EGL_NO_CONTEXT, EGL_LINUX_DMA_BUF_EXT, NULL, a);
  if (eimg == EGL_NO_IMAGE_KHR) DIE("eglCreateImageKHR failed: 0x%x", eglGetError());
  printf("egl import: pitch %u, modifier %s: ok\n", pitch, explicit_linear ? "LINEAR" : "implicit");

  int external = strcmp(bind, "external") == 0;
  GLenum target = external ? GL_TEXTURE_EXTERNAL_OES : GL_TEXTURE_2D;
  GLuint tex, dst, fbo;
  glGenTextures(1, &tex); glBindTexture(target, tex);
  if (strcmp(bind, "storage") == 0) {
    PFNTEXSTORAGE texStorage = (PFNTEXSTORAGE)eglGetProcAddress("glEGLImageTargetTexStorageEXT");
    if (!texStorage) DIE("no glEGLImageTargetTexStorageEXT");
    texStorage(target, eimg, NULL);
  } else {
    PFNGLEGLIMAGETARGETTEXTURE2DOESPROC targetTexture = (void *)eglGetProcAddress("glEGLImageTargetTexture2DOES");
    targetTexture(target, eimg);
  }
  GLenum bind_err = glGetError();
  if (bind_err) { printf("bind %s: GL error 0x%x (vrend does not check, and samples an empty texture)\n", bind, bind_err); return 3; }
  printf("bind %s: ok\n", bind);
  glTexParameteri(target, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
  glTexParameteri(target, GL_TEXTURE_MAG_FILTER, GL_NEAREST);

  // Draw the import into an ordinary RGBA texture, as a compositor would, and read that back.
  glGenTextures(1, &dst); glBindTexture(GL_TEXTURE_2D, dst);
  glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA, W, H, 0, GL_RGBA, GL_UNSIGNED_BYTE, NULL);
  glGenFramebuffers(1, &fbo); glBindFramebuffer(GL_FRAMEBUFFER, fbo);
  glFramebufferTexture2D(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_TEXTURE_2D, dst, 0);
  if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE) DIE("framebuffer incomplete");
  const char *vs = "attribute vec2 p; varying vec2 t; void main() { t = p * 0.5 + 0.5; gl_Position = vec4(p, 0.0, 1.0); }";
  const char *fs = external
    ? "#extension GL_OES_EGL_image_external : require\nprecision highp float; varying vec2 t; uniform samplerExternalOES s; void main() { gl_FragColor = texture2D(s, t); }"
    : "precision highp float; varying vec2 t; uniform sampler2D s; void main() { gl_FragColor = texture2D(s, t); }";
  GLuint prog = glCreateProgram();
  const char *srcs[2] = { vs, fs }; GLenum kinds[2] = { GL_VERTEX_SHADER, GL_FRAGMENT_SHADER };
  for (int i = 0; i < 2; i++) {
    GLuint sh = glCreateShader(kinds[i]); glShaderSource(sh, 1, &srcs[i], NULL); glCompileShader(sh);
    GLint ok; glGetShaderiv(sh, GL_COMPILE_STATUS, &ok); if (!ok) DIE("shader compile failed");
    glAttachShader(prog, sh);
  }
  glBindAttribLocation(prog, 0, "p"); glLinkProgram(prog); glUseProgram(prog);
  static const float quad[] = { -1, -1, 1, -1, -1, 1, 1, 1 };
  glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE, 0, quad); glEnableVertexAttribArray(0);
  glActiveTexture(GL_TEXTURE0); glBindTexture(target, tex);
  glUniform1i(glGetUniformLocation(prog, "s"), 0);
  glViewport(0, 0, W, H);
  glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);
  GLCHECK("draw");
  uint8_t *rb = malloc((size_t)W * H * 4);
  glReadPixels(0, 0, W, H, GL_RGBA, GL_UNSIGNED_BYTE, rb);
  GLCHECK("glReadPixels");

  uint64_t bad = 0; long first = -1;
  for (uint32_t y = 0; y < H; y++) for (uint32_t x = 0; x < W; x++) {
    uint8_t *px = rb + ((size_t)y * W + x) * 4;
    uint32_t e = pattern(x, y);
    if (px[0] != ((e >> 16) & 0xff) || px[1] != ((e >> 8) & 0xff) || px[2] != (e & 0xff)) { bad++; if (first < 0) first = (long)y * W + x; }
  }
  printf("readback: %llu of %llu pixels wrong\n", (unsigned long long)bad, (unsigned long long)W * H);
  if (bad) {
    printf("first wrong pixel at (%ld,%ld)\n", first % W, first / W);
    uint32_t rows[] = { 1, 2, 8, 9, H / 2 };
    for (int i = 0; i < 5; i++) {
      uint32_t y = rows[i]; uint8_t *px = rb + (size_t)y * W * 4;
      uint32_t sx = px[0] | (px[2] >> 4) << 8, sy = px[1] | (px[2] & 0xf) << 8;
      unsigned long long src = (unsigned long long)sy * pitch + (unsigned long long)sx * 4;
      printf("  row %u starts with source (%u,%u): byte %llu, implied pitch %.1f\n", y, sx, sy, src, (double)src / y);
    }
  }
  return bad ? 2 : 0;
}
