#!/usr/bin/env bash
set -euo pipefail

OUT="${1:-$HOME/fc-artifacts}"
mkdir -p "$OUT"
cd "$OUT"

sudo apt-get update
sudo apt-get install -y wget curl jq squashfs-tools e2fsprogs openssh-client

ARCH="x86_64"
CI_VERSION="v1.14"

release_url="https://github.com/firecracker-microvm/firecracker/releases"
latest="$(basename "$(curl -fsSLI -o /dev/null -w %{url_effective} ${release_url}/latest)")"
curl -fL "${release_url}/download/${latest}/firecracker-${latest}-${ARCH}.tgz" | tar -xz
mv "release-${latest}-${ARCH}/firecracker-${latest}-${ARCH}" ./firecracker
chmod +x ./firecracker

KERNEL_KEY=$(
  curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-&list-type=2" \
  | grep -oE "firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-[0-9]+\.[0-9]+\.[0-9]+" \
  | sort -V | tail -1 | tr -d '\r\n'
)
curl -fL "https://s3.amazonaws.com/spec.ccfc.min/${KERNEL_KEY}" -o vmlinux

UBUNTU_KEY=$(
  curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-&list-type=2" \
  | grep -oE "firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-[0-9]+\.[0-9]+\.squashfs" \
  | sort -V | tail -1 | tr -d '\r\n'
)
ubuntu_version="$(basename "${UBUNTU_KEY}" .squashfs | grep -oE '[0-9]+\.[0-9]+')"
curl -fL "https://s3.amazonaws.com/spec.ccfc.min/${UBUNTU_KEY}" -o "ubuntu-${ubuntu_version}.squashfs.upstream"

rm -rf squashfs-root
unsquashfs "ubuntu-${ubuntu_version}.squashfs.upstream"

ssh-keygen -f id_rsa -N ""
sudo mkdir -p squashfs-root/root/.ssh
sudo cp -v id_rsa.pub squashfs-root/root/.ssh/authorized_keys
mv -v id_rsa "./ubuntu-${ubuntu_version}.id_rsa"

sudo chown -R root:root squashfs-root
truncate -s 1G "ubuntu-${ubuntu_version}.ext4"
sudo mkfs.ext4 -d squashfs-root -F "ubuntu-${ubuntu_version}.ext4"

echo "Artifacts in: $OUT"
ls -1
