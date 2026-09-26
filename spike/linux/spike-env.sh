# Source to run the spike against extracted Arch QEMU 11.1.1-4 GL/UI modules
# and virglrenderer 1.3.0 without installing them system-wide.
pkgs=${SPIKE_DIR:-/data/try-omarchy-linux-spike}/archpkgs
export QEMU_MODULE_DIR=$pkgs/qemu-modules
export LD_LIBRARY_PATH=$pkgs/root/usr/lib
export RENDER_SERVER_EXEC_PATH=$pkgs/root/usr/lib/virgl_render_server
