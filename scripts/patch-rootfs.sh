#!/usr/bin/env bash
set -euo pipefail

ROOTFS="${1:?usage: $0 /path/to/ubuntu.ext4}"
MNT="${MNT:-/mnt/fcroot}"

sudo mkdir -p "$MNT"
sudo mount -o loop "$ROOTFS" "$MNT"

sudo install -m 0755 guest/fc_task.sh "$MNT/usr/local/bin/fc_task.sh"
sudo install -m 0644 guest/fc-task.service "$MNT/etc/systemd/system/fc-task.service"

sudo chroot "$MNT" systemctl enable fc-task.service >/dev/null 2>&1 || true

sudo umount "$MNT"
echo "patched rootfs: $ROOTFS"
