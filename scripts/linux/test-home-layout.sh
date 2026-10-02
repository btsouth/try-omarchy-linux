#!/usr/bin/env bash
# Exercise the real /home alias in a private filesystem, without changing the
# host. Run as root when the host restricts unprivileged mount namespaces.
set -euo pipefail
binary=$(realpath "${1:?usage: test-home-layout.sh TEST_BINARY}")
for target in var/home /var/home; do
  bwrap --unshare-pid --unshare-ipc --unshare-uts --clearenv \
    --ro-bind /usr /usr --ro-bind /lib /lib --ro-bind /lib64 /lib64 \
    --proc /proc --dev /dev --tmpfs /tmp \
    --dir /var/home/tester --symlink "$target" /home \
    --ro-bind "$binary" /test \
    --setenv HOME /home/tester --setenv XDG_CACHE_HOME /home/tester/.cache \
    --setenv TRYOMARCHY_ATOMIC_HOME_TEST 1 \
    /test -test.run '^TestLinuxAtomicHomePaths$' -test.v
done
# A matching /home alias must not permit another link at its destination.
bwrap --unshare-pid --unshare-ipc --unshare-uts --clearenv \
  --ro-bind /usr /usr --ro-bind /lib /lib --ro-bind /lib64 /lib64 \
  --proc /proc --dev /dev --tmpfs /tmp \
  --dir /outside/tester --dir /var --symlink /outside /var/home \
  --symlink var/home /home --ro-bind "$binary" /test \
  --setenv HOME /home/tester --setenv XDG_CACHE_HOME /home/tester/.cache \
  --setenv TRYOMARCHY_ATOMIC_HOME_REJECT_TEST 1 \
  /test -test.run '^TestLinuxAtomicHomeRejectsLinkedDestination$' -test.v
