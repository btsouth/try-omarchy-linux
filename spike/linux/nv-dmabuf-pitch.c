// Standalone check of how an EGL implementation honors
// EGL_DMA_BUF_PLANE0_PITCH_EXT for a LINEAR XRGB8888 dma-buf, without QEMU.
//
//   nv-dmabuf-pitch vulkan|udmabuf WIDTH HEIGHT PITCH es|gl storage|target|external [implicit|linear]
//
//   vulkan    a dma-buf exported VkBuffer on the same GPU, rows copied in at
//             PITCH: what Mesa's prime blit hands virglrenderer for Venus
//   udmabuf   CPU memory through /dev/udmabuf, written via the dma-buf mmap
//             with DMA_BUF_IOCTL_SYNC. On NVIDIA 615.71.09 this also shows
//             occasional wrong readbacks at a correct pitch (about 1 run in
//             10), a separate effect; use vulkan for deterministic results.
//   es        OpenGL ES 3.2 context
//   gl        OpenGL 4.6 core context (what virglrenderer runs on under QEMU)
//   storage   glEGLImageTargetTexStorageEXT(GL_TEXTURE_2D)
//   target    glEGLImageTargetTexture2DOES(GL_TEXTURE_2D)
//   external  glEGLImageTargetTexture2DOES(GL_TEXTURE_EXTERNAL_OES)
//   implicit  no modifier attributes (default); linear: DRM_FORMAT_MOD_LINEAR
//
// Each pixel encodes its own coordinates. The texture is drawn into an RGBA8
// renderbuffer-backed FBO and read back; any mismatch is reported with the
// row pitch the sampler actually used.
//
// Build: cc -O1 -o nv-dmabuf-pitch nv-dmabuf-pitch.c -lvulkan -lEGL -lGLESv2
#define _GNU_SOURCE
#include <fcntl.h>
#include <linux/dma-buf.h>
#include <linux/udmabuf.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/mman.h>
#include <unistd.h>
#include <vulkan/vulkan.h>
#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <GLES3/gl32.h>
#include <GLES2/gl2ext.h>
#include <libdrm/drm_fourcc.h>

#define DIE(...) do { fprintf(stderr, __VA_ARGS__); fputc('\n', stderr); exit(1); } while (0)
#define VK(x) do { VkResult r_ = (x); if (r_ != VK_SUCCESS) DIE("%s failed: %d", #x, r_); } while (0)

typedef void (*PFNTEXSTORAGE)(GLenum target, GLeglImageOES image, const GLint *attribs);

static uint32_t pattern(uint32_t x, uint32_t y) {
  // XRGB8888 little-endian: B, G, R, X. R = x low byte, G = y low byte,
  // B = x high nibble << 4 | y high nibble.
  return 0xffu << 24 | (x & 0xff) << 16 | (y & 0xff) << 8 | ((x >> 8) & 0xf) << 4 | ((y >> 8) & 0xf);
}

static int has_ext(const char *name) {
  GLint n = 0;
  glGetIntegerv(GL_NUM_EXTENSIONS, &n);
  for (GLint i = 0; i < n; i++)
    if (strcmp((const char *)glGetStringi(GL_EXTENSIONS, i), name) == 0) return 1;
  return 0;
}

static int make_dmabuf(uint32_t w, uint32_t h, uint32_t pitch) {
  size_t size = ((size_t)pitch * h + 4095) & ~(size_t)4095;
  int memfd = memfd_create("pitch-test", MFD_ALLOW_SEALING);
  if (memfd < 0 || ftruncate(memfd, size) < 0) DIE("memfd failed");
  if (fcntl(memfd, F_ADD_SEALS, F_SEAL_SHRINK) < 0) DIE("seal failed");
  int dev = open("/dev/udmabuf", O_RDWR);
  if (dev < 0) DIE("cannot open /dev/udmabuf");
  struct udmabuf_create req = { .memfd = memfd, .flags = UDMABUF_FLAGS_CLOEXEC, .offset = 0, .size = size };
  int fd = ioctl(dev, UDMABUF_CREATE, &req);
  if (fd < 0) DIE("UDMABUF_CREATE failed");
  // Write through the dma-buf itself, bracketed by the cache-sync ioctls a
  // producer must use, so the importer sees exactly these bytes.
  uint8_t *map = mmap(NULL, size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
  if (map == MAP_FAILED) DIE("mmap failed");
  struct dma_buf_sync sync = { .flags = DMA_BUF_SYNC_START | DMA_BUF_SYNC_WRITE };
  if (ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync) < 0) DIE("DMA_BUF_IOCTL_SYNC start failed");
  memset(map, 0x55, size);
  for (uint32_t y = 0; y < h; y++)
    for (uint32_t x = 0; x < w; x++) ((uint32_t *)(map + (size_t)y * pitch))[x] = pattern(x, y);
  sync.flags = DMA_BUF_SYNC_END | DMA_BUF_SYNC_WRITE;
  if (ioctl(fd, DMA_BUF_IOCTL_SYNC, &sync) < 0) DIE("DMA_BUF_IOCTL_SYNC end failed");
  munmap(map, size);
  return fd;
}

static int find_mem(VkPhysicalDevice pd, uint32_t bits, VkMemoryPropertyFlags want) {
  VkPhysicalDeviceMemoryProperties mp;
  vkGetPhysicalDeviceMemoryProperties(pd, &mp);
  for (uint32_t i = 0; i < mp.memoryTypeCount; i++)
    if ((bits & (1u << i)) && (mp.memoryTypes[i].propertyFlags & want) == want) return (int)i;
  DIE("no memory type with flags 0x%x", want);
}

static int make_vulkan_dmabuf(uint32_t w, uint32_t h, uint32_t pitch) {
  VkApplicationInfo app = { .sType = VK_STRUCTURE_TYPE_APPLICATION_INFO, .apiVersion = VK_API_VERSION_1_2 };
  VkInstanceCreateInfo ici = { .sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO, .pApplicationInfo = &app };
  VkInstance inst; VK(vkCreateInstance(&ici, NULL, &inst));
  uint32_t n = 8; VkPhysicalDevice pds[8], pd = VK_NULL_HANDLE; VK(vkEnumeratePhysicalDevices(inst, &n, pds));
  VkPhysicalDeviceProperties props;
  for (uint32_t i = 0; i < n && !pd; i++) {
    vkGetPhysicalDeviceProperties(pds[i], &props);
    if (props.deviceType == VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU || props.deviceType == VK_PHYSICAL_DEVICE_TYPE_INTEGRATED_GPU) pd = pds[i];
  }
  if (!pd) DIE("no Vulkan GPU");
  printf("Vulkan source: %s\n", props.deviceName);
  float prio = 1;
  VkDeviceQueueCreateInfo qci = { .sType = VK_STRUCTURE_TYPE_DEVICE_QUEUE_CREATE_INFO, .queueCount = 1, .pQueuePriorities = &prio };
  const char *exts[] = { VK_KHR_EXTERNAL_MEMORY_FD_EXTENSION_NAME, VK_EXT_EXTERNAL_MEMORY_DMA_BUF_EXTENSION_NAME };
  VkDeviceCreateInfo dci = { .sType = VK_STRUCTURE_TYPE_DEVICE_CREATE_INFO, .queueCreateInfoCount = 1, .pQueueCreateInfos = &qci,
                             .enabledExtensionCount = 2, .ppEnabledExtensionNames = exts };
  VkDevice dev; VK(vkCreateDevice(pd, &dci, NULL, &dev));
  VkQueue q; vkGetDeviceQueue(dev, 0, 0, &q);

  // Pattern in a host-visible staging buffer, tightly packed.
  VkBufferCreateInfo sbci = { .sType = VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO, .size = (VkDeviceSize)w * h * 4, .usage = VK_BUFFER_USAGE_TRANSFER_SRC_BIT };
  VkBuffer staging; VK(vkCreateBuffer(dev, &sbci, NULL, &staging));
  VkMemoryRequirements mr; vkGetBufferMemoryRequirements(dev, staging, &mr);
  VkMemoryAllocateInfo smai = { .sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO, .allocationSize = mr.size,
    .memoryTypeIndex = find_mem(pd, mr.memoryTypeBits, VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | VK_MEMORY_PROPERTY_HOST_COHERENT_BIT) };
  VkDeviceMemory smem; VK(vkAllocateMemory(dev, &smai, NULL, &smem)); VK(vkBindBufferMemory(dev, staging, smem, 0));
  uint32_t *p; VK(vkMapMemory(dev, smem, 0, VK_WHOLE_SIZE, 0, (void **)&p));
  for (uint32_t y = 0; y < h; y++) for (uint32_t x = 0; x < w; x++) p[y * w + x] = pattern(x, y);
  vkUnmapMemory(dev, smem);

  // Exportable destination with PITCH-byte rows, like Mesa's prime blit buffer.
  VkExternalMemoryBufferCreateInfo emb = { .sType = VK_STRUCTURE_TYPE_EXTERNAL_MEMORY_BUFFER_CREATE_INFO,
                                           .handleTypes = VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT };
  VkBufferCreateInfo bci = { .sType = VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO, .pNext = &emb,
                             .size = ((VkDeviceSize)pitch * h + 4095) & ~(VkDeviceSize)4095, .usage = VK_BUFFER_USAGE_TRANSFER_DST_BIT };
  VkBuffer buf; VK(vkCreateBuffer(dev, &bci, NULL, &buf));
  vkGetBufferMemoryRequirements(dev, buf, &mr);
  VkMemoryDedicatedAllocateInfo ded = { .sType = VK_STRUCTURE_TYPE_MEMORY_DEDICATED_ALLOCATE_INFO, .buffer = buf };
  VkExportMemoryAllocateInfo exp = { .sType = VK_STRUCTURE_TYPE_EXPORT_MEMORY_ALLOCATE_INFO, .pNext = &ded,
                                     .handleTypes = VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT };
  VkMemoryAllocateInfo mai = { .sType = VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO, .pNext = &exp, .allocationSize = mr.size,
                               .memoryTypeIndex = find_mem(pd, mr.memoryTypeBits, VK_MEMORY_PROPERTY_DEVICE_LOCAL_BIT) };
  VkDeviceMemory mem; VK(vkAllocateMemory(dev, &mai, NULL, &mem)); VK(vkBindBufferMemory(dev, buf, mem, 0));

  VkCommandPoolCreateInfo cpci = { .sType = VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO };
  VkCommandPool pool; VK(vkCreateCommandPool(dev, &cpci, NULL, &pool));
  VkCommandBufferAllocateInfo cbai = { .sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO, .commandPool = pool,
                                       .level = VK_COMMAND_BUFFER_LEVEL_PRIMARY, .commandBufferCount = 1 };
  VkCommandBuffer cb; VK(vkAllocateCommandBuffers(dev, &cbai, &cb));
  VkCommandBufferBeginInfo cbbi = { .sType = VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO };
  VK(vkBeginCommandBuffer(cb, &cbbi));
  VkBufferCopy *rows = calloc(h, sizeof *rows);
  for (uint32_t y = 0; y < h; y++) rows[y] = (VkBufferCopy){ (VkDeviceSize)y * w * 4, (VkDeviceSize)y * pitch, (VkDeviceSize)w * 4 };
  vkCmdCopyBuffer(cb, staging, buf, h, rows);
  VK(vkEndCommandBuffer(cb));
  VkSubmitInfo si = { .sType = VK_STRUCTURE_TYPE_SUBMIT_INFO, .commandBufferCount = 1, .pCommandBuffers = &cb };
  VK(vkQueueSubmit(q, 1, &si, VK_NULL_HANDLE));
  VK(vkQueueWaitIdle(q));

  PFN_vkGetMemoryFdKHR getMemoryFd = (PFN_vkGetMemoryFdKHR)vkGetDeviceProcAddr(dev, "vkGetMemoryFdKHR");
  VkMemoryGetFdInfoKHR gfi = { .sType = VK_STRUCTURE_TYPE_MEMORY_GET_FD_INFO_KHR, .memory = mem,
                               .handleType = VK_EXTERNAL_MEMORY_HANDLE_TYPE_DMA_BUF_BIT_EXT };
  int fd; VK(getMemoryFd(dev, &gfi, &fd));
  return fd;  // Vulkan objects stay alive until exit
}

int main(int argc, char **argv) {
  if (argc != 7 && argc != 8)
    DIE("usage: %s vulkan|udmabuf WIDTH HEIGHT PITCH es|gl storage|target|external [implicit|linear]", argv[0]);
  int use_vulkan = strcmp(argv[1], "vulkan") == 0;
  uint32_t W = atoi(argv[2]), H = atoi(argv[3]), pitch = atoi(argv[4]);
  int desktop = strcmp(argv[5], "gl") == 0;
  const char *bind = argv[6];
  int explicit_linear = argc == 8 && strcmp(argv[7], "linear") == 0;
  int external = strcmp(bind, "external") == 0;
  if (pitch < W * 4) DIE("pitch must be at least width * 4");

  int fd = use_vulkan ? make_vulkan_dmabuf(W, H, pitch) : make_dmabuf(W, H, pitch);

  PFNEGLQUERYDEVICESEXTPROC queryDevices = (void *)eglGetProcAddress("eglQueryDevicesEXT");
  PFNEGLQUERYDEVICESTRINGEXTPROC queryDeviceString = (void *)eglGetProcAddress("eglQueryDeviceStringEXT");
  PFNEGLGETPLATFORMDISPLAYEXTPROC getPlatformDisplay = (void *)eglGetProcAddress("eglGetPlatformDisplayEXT");
  EGLDeviceEXT devs[8]; EGLint ndev = 0;
  if (!queryDevices || !queryDevices(8, devs, &ndev) || ndev == 0) DIE("no EGL devices");
  EGLDisplay dpy = EGL_NO_DISPLAY;
  for (EGLint i = 0; i < ndev && dpy == EGL_NO_DISPLAY; i++) {
    const char *exts = queryDeviceString(devs[i], EGL_EXTENSIONS);
    if (exts && strstr(exts, "EGL_MESA_device_software")) continue;  // want the hardware device
    dpy = getPlatformDisplay(EGL_PLATFORM_DEVICE_EXT, devs[i], NULL);
  }
  if (dpy == EGL_NO_DISPLAY || !eglInitialize(dpy, NULL, NULL)) DIE("eglInitialize failed");
  if (!strstr(eglQueryString(dpy, EGL_EXTENSIONS), "EGL_EXT_image_dma_buf_import")) DIE("no EGL_EXT_image_dma_buf_import");

  eglBindAPI(desktop ? EGL_OPENGL_API : EGL_OPENGL_ES_API);
  EGLint cattr_es[] = { EGL_CONTEXT_MAJOR_VERSION, 3, EGL_CONTEXT_MINOR_VERSION, 2, EGL_NONE };
  EGLint cattr_gl[] = { EGL_CONTEXT_MAJOR_VERSION, 4, EGL_CONTEXT_MINOR_VERSION, 6,
                        EGL_CONTEXT_OPENGL_PROFILE_MASK, EGL_CONTEXT_OPENGL_CORE_PROFILE_BIT, EGL_NONE };
  EGLContext ctx = eglCreateContext(dpy, EGL_NO_CONFIG_KHR, EGL_NO_CONTEXT, desktop ? cattr_gl : cattr_es);
  if (ctx == EGL_NO_CONTEXT || !eglMakeCurrent(dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, ctx)) DIE("cannot make a surfaceless context");
  printf("EGL %s %s; GL %s; %s\n", eglQueryString(dpy, EGL_VENDOR), eglQueryString(dpy, EGL_VERSION),
         glGetString(GL_VERSION), glGetString(GL_RENDERER));

  PFNEGLQUERYDMABUFMODIFIERSEXTPROC queryMods = (void *)eglGetProcAddress("eglQueryDmaBufModifiersEXT");
  if (queryMods) {
    EGLuint64KHR mods[64]; EGLBoolean ext_only[64]; EGLint nm = 0;
    queryMods(dpy, DRM_FORMAT_XRGB8888, 64, mods, ext_only, &nm);
    for (EGLint i = 0; i < nm; i++)
      if (mods[i] == DRM_FORMAT_MOD_LINEAR) printf("eglQueryDmaBufModifiersEXT: XRGB8888 LINEAR external_only=%d\n", ext_only[i]);
  }

  const char *need = strcmp(bind, "storage") == 0 ? "GL_EXT_EGL_image_storage" : external ? "GL_OES_EGL_image_external" : "GL_OES_EGL_image";
  if (!has_ext(need)) DIE("%s is not advertised in this context; not testing it", need);
  if (external && !desktop && !has_ext("GL_OES_EGL_image_external_essl3")) DIE("GL_OES_EGL_image_external_essl3 not advertised");

  EGLint a[32]; int k = 0;
  a[k++] = EGL_WIDTH; a[k++] = W; a[k++] = EGL_HEIGHT; a[k++] = H;
  a[k++] = EGL_LINUX_DRM_FOURCC_EXT; a[k++] = DRM_FORMAT_XRGB8888;
  a[k++] = EGL_DMA_BUF_PLANE0_FD_EXT; a[k++] = fd;
  a[k++] = EGL_DMA_BUF_PLANE0_OFFSET_EXT; a[k++] = 0;
  a[k++] = EGL_DMA_BUF_PLANE0_PITCH_EXT; a[k++] = pitch;
  if (explicit_linear) {
    a[k++] = EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT; a[k++] = 0;
    a[k++] = EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT; a[k++] = 0;
  }
  a[k++] = EGL_NONE;
  PFNEGLCREATEIMAGEKHRPROC createImage = (void *)eglGetProcAddress("eglCreateImageKHR");
  EGLImageKHR img = createImage(dpy, EGL_NO_CONTEXT, EGL_LINUX_DMA_BUF_EXT, NULL, a);
  if (img == EGL_NO_IMAGE_KHR) DIE("eglCreateImageKHR failed: 0x%x", eglGetError());

  GLenum target = external ? GL_TEXTURE_EXTERNAL_OES : GL_TEXTURE_2D;
  GLuint tex; glGenTextures(1, &tex); glBindTexture(target, tex);
  if (strcmp(bind, "storage") == 0)
    ((PFNTEXSTORAGE)eglGetProcAddress("glEGLImageTargetTexStorageEXT"))(target, img, NULL);
  else
    ((PFNGLEGLIMAGETARGETTEXTURE2DOESPROC)eglGetProcAddress("glEGLImageTargetTexture2DOES"))(target, img);
  GLenum err = glGetError();
  printf("%ux%u pitch %u (%s modifier), %s: ", W, H, pitch, explicit_linear ? "LINEAR" : "implicit", bind);
  if (err) { printf("GL error 0x%x\n", err); return 3; }
  glTexParameteri(target, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
  glTexParameteri(target, GL_TEXTURE_MAG_FILTER, GL_NEAREST);

  GLuint rb, fbo;
  glGenRenderbuffers(1, &rb); glBindRenderbuffer(GL_RENDERBUFFER, rb);
  glRenderbufferStorage(GL_RENDERBUFFER, GL_RGBA8, W, H);
  glGenFramebuffers(1, &fbo); glBindFramebuffer(GL_FRAMEBUFFER, fbo);
  glFramebufferRenderbuffer(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_RENDERBUFFER, rb);
  if (glCheckFramebufferStatus(GL_FRAMEBUFFER) != GL_FRAMEBUFFER_COMPLETE) DIE("framebuffer incomplete");

  const char *hdr = desktop ? "#version 330 core\n" : "#version 300 es\n";
  const char *ext = external ? (desktop ? "#extension GL_OES_EGL_image_external : require\n"
                                        : "#extension GL_OES_EGL_image_external_essl3 : require\n") : "";
  char vs[256], fs[512];
  snprintf(vs, sizeof vs, "%sin vec2 p; out vec2 t; void main() { t = p * 0.5 + 0.5; gl_Position = vec4(p, 0.0, 1.0); }", hdr);
  snprintf(fs, sizeof fs, "%s%sprecision highp float; in vec2 t; out vec4 c; uniform %s s; void main() { c = texture(s, t); }",
           hdr, ext, external ? "samplerExternalOES" : "sampler2D");
  GLuint prog = glCreateProgram();
  const char *srcs[2] = { vs, fs }; GLenum kinds[2] = { GL_VERTEX_SHADER, GL_FRAGMENT_SHADER };
  for (int i = 0; i < 2; i++) {
    GLuint sh = glCreateShader(kinds[i]); glShaderSource(sh, 1, &srcs[i], NULL); glCompileShader(sh);
    GLint ok; glGetShaderiv(sh, GL_COMPILE_STATUS, &ok);
    if (!ok) { char log[1024]; glGetShaderInfoLog(sh, sizeof log, NULL, log); DIE("shader: %s", log); }
    glAttachShader(prog, sh);
  }
  glBindAttribLocation(prog, 0, "p"); glLinkProgram(prog); glUseProgram(prog);
  static const float quad[] = { -1, -1, 1, -1, -1, 1, 1, 1 };
  GLuint vao, vbo;
  glGenVertexArrays(1, &vao); glBindVertexArray(vao);
  glGenBuffers(1, &vbo); glBindBuffer(GL_ARRAY_BUFFER, vbo);
  glBufferData(GL_ARRAY_BUFFER, sizeof quad, quad, GL_STATIC_DRAW);
  glVertexAttribPointer(0, 2, GL_FLOAT, GL_FALSE, 0, 0); glEnableVertexAttribArray(0);
  glActiveTexture(GL_TEXTURE0); glBindTexture(target, tex);
  glUniform1i(glGetUniformLocation(prog, "s"), 0);
  glViewport(0, 0, W, H);
  glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);
  uint8_t *out = malloc((size_t)W * H * 4);
  glReadPixels(0, 0, W, H, GL_RGBA, GL_UNSIGNED_BYTE, out);
  if ((err = glGetError())) DIE("GL error 0x%x after draw", err);

  uint64_t bad = 0;
  for (uint32_t y = 0; y < H; y++)
    for (uint32_t x = 0; x < W; x++) {
      uint8_t *px = out + ((size_t)y * W + x) * 4;
      uint32_t e = pattern(x, y);
      if (px[0] != ((e >> 16) & 0xff) || px[1] != ((e >> 8) & 0xff) || px[2] != (e & 0xff)) bad++;
    }
  if (!bad) { printf("correct\n"); return 0; }
  // Row 8's first texel tells which byte offset the sampler read for it.
  uint8_t *px = out + (size_t)8 * W * 4;
  uint32_t sx = px[0] | (px[2] >> 4) << 8, sy = px[1] | (px[2] & 0xf) << 8;
  printf("WRONG, %llu of %u pixels; row 8 starts at source (%u,%u), so the sampler used pitch %.0f\n",
         (unsigned long long)bad, W * H, sx, sy, ((double)sy * pitch + sx * 4) / 8);
  return 2;
}
