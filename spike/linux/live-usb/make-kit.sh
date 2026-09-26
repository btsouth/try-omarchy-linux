#!/usr/bin/env bash
# Stage the live-USB hardware kit into DEST, a directory on a second USB stick
# or external drive with at least 16 GB free (exFAT or ext4; FAT32 cannot hold
# the 7.5 GB rootfs). Run on the machine that built the spike Flatpak.
#   make-kit.sh DEST
set -euo pipefail
dest=${1:?usage: make-kit.sh DEST}
here=$(cd "$(dirname "$0")" && pwd)
spike=${SPIKE_DIR:-/data/try-omarchy-linux-spike}

mkdir -p "$dest/guest"
cp "$here/kit.sh" "$here/kit-qemu.sh" "$here/guest-kit.sh" "$here/CHECKLIST.md" "$dest/"
cp "$here/../boot.sh" "$here/../guest-ssh.sh" "$here/../stop.sh" "$dest/"
cp "$spike/flatpak/com.tryomarchy.TryOmarchy.flatpak" "$dest/"

for f in vmlinuz-linux initramfs-linux.img build-spec.json SHA256SUMS; do
  cp "$spike/guest/$f" "$dest/guest/"
done
echo "copying the 7.5 GB rootfs..."
cp --sparse=always "$spike/guest/rootfs.ext4" "$dest/guest/"
(cd "$dest/guest" &&
  grep -E ' (vmlinuz-linux|initramfs-linux.img|build-spec.json|rootfs.ext4)$' SHA256SUMS | sha256sum -c -)

# A throwaway key for the disposable guest's SSH; it opens nothing else.
[ -f "$dest/spike_key" ] || ssh-keygen -q -t ed25519 -N '' -C try-omarchy-live-kit -f "$dest/spike_key"
chmod +x "$dest"/*.sh 2>/dev/null || true
sync
echo "kit ready in $dest"
