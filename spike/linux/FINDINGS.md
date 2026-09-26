# Linux host spike: Phase 0 findings

September 26, 2026. Goal: confirm or kill the risky assumptions behind a Linux
version of Try Omarchy before refactoring anything. Everything here ran on one
machine; results that depend on the GPU vendor or desktop are marked. The AMD
and GNOME half is packaged as a live USB kit (`live-usb/`) and not run yet.

## Test setup

- Host: Omarchy (Linux 7.2.5, Hyprland), NVIDIA GeForce RTX 4070 SUPER on the
  proprietary 615.71.09 driver, 28 threads, 31 GiB RAM.
- Guest: the unmodified public v0.3.0 release image (`vmlinuz-linux`,
  `initramfs-linux.img`, `rootfs.ext4.zst`), checksums verified against the
  release's `SHA256SUMS`, behind a disposable qcow2 overlay. Guest Mesa is
  26.2.3.
- QEMU, two builds:
  - Arch's `qemu-system-x86` 11.1.1-4, with the matching GL, SDL, GTK,
    egl-headless, D-Bus and PipeWire modules and `virglrenderer` 1.3.0-2
    extracted into a private directory (`spike-env.sh`) rather than installed.
  - The spike Flatpak (`flatpak/com.tryomarchy.TryOmarchy.yml`): upstream QEMU
    11.1.1, virglrenderer 1.3.0 with Venus and libslirp 4.9.1 on the
    Freedesktop 25.08 runtime, no Try Omarchy patches (`flatpak-qemu.sh`).
- Devices and kernel command line mirror `app/qemu.go`, with WHPX swapped for
  KVM (`boot.sh`). SSH is enabled through `tryomarchy.sshd=1` and a throwaway
  key in `tryomarchy.sshkey=` so the guest can be inspected (`guest-ssh.sh`).
- Every window ran inside an omabox (contained Hyprland + Omarchy desktop).
  QEMU itself runs on the host because a box has no `/dev/kvm`; `in-box.sh`
  points its Wayland connection at the box and its session bus at nothing.

## Results

| Question | Result |
|---|---|
| Does the release image boot under KVM unchanged? | Yes. SSH in 12-14 s, Hyprland desktop shortly after. CI already boots it under KVM headless. |
| OpenGL on the host GPU (virgl) | Works on NVIDIA. Guest Hyprland: `Renderer: virgl (NVIDIA GeForce RTX 4070 SUPER/PCIe/SSE2)`; core, compat and ES profiles; GLES gears render correctly. |
| Vulkan on the host GPU (Venus) | Renders correctly on NVIDIA, but **every presented frame is sheared** unless its rows happen to be a multiple of 256 bytes. Root cause is an NVIDIA GL bug reached through virglrenderer, already reported upstream; a small Venus patch works around it (below). |
| Which device do Vulkan apps get? | **The host's llvmpipe (CPU), by default.** The render server saw the runtime's llvmpipe, and Mesa's device-select layer in the guest prefers it. Fixed from the launcher (below). |
| Idle cost | QEMU at about 6% of one core with the desktop idle and an SDL window presenting at 60 Hz, measured in a box through the Flatpak. Same as the earlier headless figure. |
| Display backend: SDL vs GTK | **SDL.** Both follow window resizes and pass Super through when grabbed. SDL hands the guest physical pixels at 2x and 1.5x (sharp); GTK hands it logical pixels (blurry at any scale above 1) and adds a menu bar. SDL also keeps the Windows runtime's SDL patches relevant. |
| Guest resolution follows the window | Yes with SDL: tiled 1896x1032, floating 1200x700, 2x-scaled 936x492 -> guest 1872x984, 1.5x 1256x672 -> guest 1884x1008. |
| Guest scale on HiDPI hosts | Guest stays at scale 1, so its UI is half size on a 2x host. Needs a host-to-guest scale bridge (the Mac app has one). |
| Super key | Without a grab, Super+Space opens the host's menu. After QEMU's grab (Ctrl+Alt+G) it opens the guest's: QEMU sets `SDL_HINT_GRAB_KEYBOARD=1` and SDL uses Wayland keyboard-shortcuts-inhibit. Product needs a focus-scoped keyboard-only grab (runtime patch). |
| Window close | With `window-close=off` the compositor's close request is ignored and nothing reports it. Needs a runtime patch that raises a QMP event so the launcher can confirm and shut down cleanly, as `closeguard.go` does on Windows. |
| Shared folder (9p) | Works both ways with stock QEMU. The guest mounts it at `/mnt/host` and links it into home under the host folder's name (from `tryomarchy.sharename`), so the product should keep calling the folder `Omarchy Shared`. First boot opens it in the file manager. Guest-written files land on the host owned by the user. |
| Free-page reporting | Works on KVM with stock QEMU: after the guest touched and freed 1.5 GiB, QEMU RSS fell from 4.63 GiB to 3.82 GiB within 10 s. No runtime patch needed (Windows needs r19's 0015). |
| Flatpak runtime | Works. Builds in a few minutes with `flatpak-builder` (only libslirp and PyYAML beyond the SDK), 25 MB bundle, 381 MB installed. KVM, virgl and Venus all run inside the sandbox; `virgl_render_server` resolves at its compiled-in `/app/libexec` path, and the runtime's NVIDIA GL extension (`GL.nvidia-615-71-09`) provides both GL and Vulkan. No "not responding" dialog at idle, under GLES or under Vulkan. |

## Venus on NVIDIA: the sheared frames

Reproduced without QEMU by `nv-dmabuf-pitch.c`, which puts a coordinate
pattern into a dma-buf exported VkBuffer at a chosen pitch, imports it with
EGL, binds it one of three ways and reads the pixels back.

How a Venus frame reaches the screen on an NVIDIA host:

1. Venus spoofs its DRM identity on NVIDIA (`vn_wsi_init`) so Mesa's WSI
   takes the prime blit path: the app renders to an optimally tiled image,
   which is copied into a linear VkBuffer with a row pitch of
   `align(width * cpp, 256)`, shared as a dma-buf with an implicit modifier.
2. The guest compositor imports that dma-buf through virgl. virglrenderer
   turns it into an EGLImage with the right pitch
   (`virgl_egl_image_from_dmabuf`) and binds it to a `GL_TEXTURE_2D` with
   `glEGLImageTargetTexStorageEXT` (`vrend_resource_alloc_texture`).
3. NVIDIA advertises LINEAR XRGB8888 as external-only. It correctly refuses
   `glEGLImageTargetTexture2DOES(GL_TEXTURE_2D)` with `GL_INVALID_OPERATION`,
   but `glEGLImageTargetTexStorageEXT` succeeds and then reads the rows at
   `align(width * cpp, 32)`, ignoring the imported pitch. Sampling the same
   import as `GL_TEXTURE_EXTERNAL_OES` honors the pitch.

So frames come out right only when `align(width * 4, 256)` equals
`align(width * 4, 32)`, i.e. widths that are a multiple of 64, and a tiled
window almost never lands on one. Deterministic: identical results over 25
runs, in GL 4.6 core and GLES 3.2 contexts, with implicit or explicit LINEAR
modifiers.

A second NVIDIA defect showed up on the way: a pitch that is not a multiple
of 32 (4004 for 1001 px) is accepted by `eglCreateImageKHR` and then read
wrong even as `GL_TEXTURE_EXTERNAL_OES`. EGL_EXT_image_dma_buf_import requires
`EGL_BAD_ACCESS` for a pitch the implementation cannot honor. Both are spec
violations: EXT_EGL_image_storage makes a successfully bound texture a
sibling of the EGLImage, and the dma-buf modifiers extension defines
external-only as usable with `GL_TEXTURE_EXTERNAL_OES` alone (both in the
Khronos registry).

With a CPU dma-buf from `/dev/udmabuf` instead of a VkBuffer, NVIDIA shows
the same pitch behavior plus occasional wrong readbacks at a correct pitch
(about 1 run in 10) that survive `DMA_BUF_IOCTL_SYNC`. That is a separate
effect whose cause is unclear, so it is kept out of the reports. Confirmed in the guest: vkgears floated at
1024x768 is clean, at 1000x700 sheared, same session. The earlier `vkcube`
case also presents FP16 (`XB4H`, 8 bytes a pixel), which the guest's Wayland
surface lists first, but the format is incidental.

Ruled out along the way: the Arch packaging (the Flatpak build behaves the
same), Venus rendering itself (`MESA_VK_WSI_DEBUG=sw`, which copies through
`wl_shm`, is clean), and the host Vulkan row pitch (NVIDIA reports the same
pitch Mesa computes).

### Fix options

| Option | Where | Cost | Status |
|---|---|---|---|
| 32-byte prime buffer (`mesa/0001-venus-32-byte-prime-stride-on-nvidia.patch`): a per-device prime stride alignment in common WSI, set to 32 by Venus when the renderer is NVIDIA | Guest Mesa | No per-frame cost; the guest image must carry a patched `vulkan-virtio` until upstream takes it | **Verified**: vkgears at 997x611, 1001x700 and tiled 1872x982 all clean. A first version used alignment 1, which only worked because every width tried was a multiple of 8. |
| `MESA_VK_WSI_DEBUG=sw` for Linux NVIDIA hosts, set from a `tryomarchy.*` flag | Guest config | CPU copy per frame: vkgears near full screen went from about 2% to about 25% guest CPU at 60 FPS | Works today, stopgap only |
| Shadow texture in virglrenderer: sample the import as external and blit into a regular texture before use | Host runtime | A GPU copy per frame; a medium patch in vrend | Not attempted |
| NVIDIA fixing `glEGLImageTargetTexStorageEXT` (reject external-only imports or honor the pitch) | Driver | None | Reported by others in March 2026 on 595.45.04, no NVIDIA reply yet |

Open questions for the Mesa patch before it ships in the shared guest image:

- It keys on the Venus *renderer* being NVIDIA, but the bug lives in the host
  *GL*. On a hybrid laptop the renderer can be the NVIDIA dGPU while QEMU's
  GL runs on the Intel or AMD iGPU, and those drivers may require an aligned
  pitch. Needs a hybrid machine, or a launcher-provided switch instead.
- Windows hosts with NVIDIA would get the tight stride too, through a
  different import path in the Windows runtime. Needs a Windows NVIDIA test
  first, or the same launcher switch.

### Upstream status

Known upstream since March 2026, and the workaround is already public:

- virglrenderer issue 651 (open): a user found the 32-byte prime stride fix
  on March 13, the Venus maintainer concluded it is an NVIDIA EGL bug and
  cc'd NVIDIA engineers. No activity since March 24, and no Venus or Mesa
  change has landed for it.
- virglrenderer MR 1605 (open) switches the fallback bind to
  `GL_TEXTURE_EXTERNAL_OES` and was reported to make no difference on NVIDIA.
  On NVIDIA vrend never reaches that line: it takes the
  `glEGLImageTargetTexStorageEXT` branch because texture storage and
  `GL_EXT_EGL_image_storage` are available. That is the one piece of new
  information we have for them (`upstream/virglrenderer-651-comment.md`,
  optional).
- NVIDIA forum thread 364360 (March 22, 595.45.04). Brandon replied there
  on September 26 with the narrowed entry point and the repro.
- Mesa issue 15149 (closed) is the same NVIDIA pitch behavior with i915
  buffers imported on NVIDIA.

The product does not wait on any of this: it carries the 32-byte Venus
workaround. On the Intel devbox, stock Mesa presents correctly at widths that
shear on NVIDIA (prime blit path, iris honors the pitch), which confirms the
bug is NVIDIA-only.

### Llvmpipe and duplicate devices

The Freedesktop runtime lists each Vulkan driver manifest in two directories,
so the render server enumerated every host GPU twice, plus the runtime's
llvmpipe. The guest saw four Venus devices, and on Wayland Mesa's
device-select layer picked Venus-on-llvmpipe for apps that do not choose a
device themselves (its DRM node matches the compositor's; the NVIDIA one is
blanked by the spoof above). vkgears ran on the host CPU while vkcube, which
picks by index, got the GPU.

Fix in the launcher: `VK_DRIVER_FILES=/usr/lib/x86_64-linux-gnu/GL/vulkan/icd.d`
and `VK_LOADER_DRIVERS_DISABLE=*lvp*` for QEMU. After that the guest sees one
Venus device plus its own llvmpipe, and default apps get the GPU. Both
`flatpak-qemu.sh` and the live USB kit set these.

### Smaller Venus notes

- `vkgears -present-mailbox` runs at 1.3 FPS with or without the fix. The
  guest's `VN_PERF=no_async_present` workaround is the likely cause; not
  investigated.
- The guest's idle policy blanks and locks the display after a few minutes.
  QEMU then shows "Display output is not active." in the window, which looks
  like a crash to a user. The Linux launcher should keep the guest awake
  while its window is visible, or show that state better.

## Crashes found, and why they do not block

Two early QEMU aborts were in NVIDIA's `libnvidia-eglcore`, both only with
Venus enabled, both with the Arch build:

1. `-display egl-headless` (GBM-backed EGL on the render node) aborted inside
   `virgl_renderer_resource_create` during guest udev. The product never uses
   egl-headless. With Venus off it boots and renders.
2. With an SDL window, QEMU aborted in `surface_gl_update_texture` from the
   text-console cursor timer (`vt100_update_cursor`). The default `-monitor vc`
   and `-parallel vc` text consoles were drawing through the shared GL context.
   `-monitor none -parallel none` removes them and the crash did not recur in
   three further Venus runs. The product should pass both.

One run also showed Hyprland's "Application Not Responding" dialog for the
QEMU window while a Vulkan and a GLES client presented at the same time. It did
not reproduce with each load alone (idle 60 s, GLES 30 s, Vulkan 30 s, SDL and
GTK), nor in any Flatpak run, but it points at the main loop blocking on Venus
fences on NVIDIA.

Venus also needs `virgl_render_server`. virglrenderer finds it by a compiled-in
path, so a relocated build must set `RENDER_SERVER_EXEC_PATH` or install it at
the configured prefix. Without it the guest's `vkCreateInstance` fails with
`ERROR_OUT_OF_HOST_MEMORY`. The Flatpak installs it at the prefix.

## Flatpak notes

- `--nosocket=wayland` also clears `WAYLAND_DISPLAY`, even one passed with
  `--env`. `flatpak-qemu.sh` passes the box socket under another name and sets
  it inside the sandbox, so the window can never fall back to the real
  session's `wayland-0`.
- Without `--no-session-bus`, SDL inside the sandbox reaches the real session
  through the portal (screensaver inhibit). The spike wrapper cuts it; the
  product wants it.
- The app ID is `com.tryomarchy.TryOmarchy`. QEMU's own `.desktop` file and
  icons are not exported under that ID; the product ships its own.

## GNOME clipboard

Confirmed: Mutter implements neither `wlr-data-control` nor
`ext-data-control-v1`. The request (mutter issue 524) was closed two hours
after it was opened in 2019, there is no merge request, and `src/wayland` on
`main` has no implementation. A background host process cannot read or set
the clipboard on GNOME Wayland. KWin, Sway and Hyprland offer data-control.

## Go app

A Linux build of `app/` stops on seven symbols the shared files take from
Windows-only files: `logf`, `config`, `getUI`, `appTitle`, `setSparse`,
`sparseCopy`, `punchHole`. The boundary between shared and Windows code is
narrow; the work is porting the orchestration in `main.go`, `setup.go`,
`fetch.go` and `ui.go`.

## Not covered here

- AMD and Intel hosts, GNOME in practice (the shortcuts-inhibit prompt,
  fractional scaling, close button, audio). `live-usb/` packages these for the
  AMD Windows test laptop booted from a Fedora or Ubuntu live USB; see
  `live-usb/CHECKLIST.md`. On AMD, Venus should take the native path with
  modifiers (`prime_blit=0` in the kit's logs), so the NVIDIA shear should not
  appear there.
- Hybrid Intel or AMD iGPU plus NVIDIA dGPU laptops (see the Mesa patch's
  open questions).
- KDE and X11 sessions (Linux Mint).
- Audio through PipeWire on this machine. The module loads; playback was left
  to the live USB run to keep the spike off the real audio session.
