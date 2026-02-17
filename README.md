# fc-metrics

Host-side Firecracker runner that emits a single JSON receipt per microVM run, including disk and (optional) network byte counters plus guest-produced workspace deltas.

## What it is

`fc-metrics` is a small wrapper around Firecracker that:

* boots a microVM
* waits for a completion marker from inside the guest (serial console)
* forces Firecracker to flush metrics
* outputs a single JSON receipt containing per-run measurements

The runner is intentionally “thin”: it coordinates Firecracker and collects metrics. Guest behavior is owned by the root filesystem image (the “agent” lives in the guest).

## What problem it solves

Firecracker metrics and VM lifecycle are easy to get wrong in production:

* Firecracker can keep running after a guest powers off, so host completion should not rely on Firecracker process exit.
* Metrics can be incomplete unless explicitly flushed and the metrics file is allowed to advance.
* A clean “done” signal is needed to reliably mark run completion.
* Teams often want one portable JSON record per run instead of scraping logs.

`fc-metrics` provides a consistent pattern for producing a per-run receipt.

## Receipt output

Typical fields:

* `run_id`, `started_at`, `ended_at`, `duration_ms`, `exit_code`
* `kernel`, `rootfs`, `firecracker_bin`
* `vcpus`, `mem_mib`
* `block_read_bytes`, `block_write_bytes`
* `net_rx_bytes`, `net_tx_bytes` (requires NIC + traffic)
* `workspace_files_delta`, `workspace_bytes_delta` (emitted by guest)
* `metrics_lines`
* `firecracker_log_path`
* optional raw metrics blob (when enabled)

## How it works

### Guest side

The guest rootfs contains:

* `fc-task.service` (systemd oneshot)
* `/usr/local/bin/fc_task.sh`

`fc_task.sh` writes output into `/workspace/run-<stamp>/`, calculates before/after stats, prints exactly one JSON line to `/dev/console`, then powers off:

```json
{"workspace_files_delta":3,"workspace_bytes_delta":5242892,"stamp":"fc_task_v4_1771326807"}
```

The host runner parses Firecracker serial output (captured in `firecracker.log`) to find this JSON line.

### Host side

The runner:

1. Starts Firecracker with an API unix socket.
2. Configures:

    * `/metrics` (metrics output path + flush interval)
    * `/machine-config`
    * `/boot-source`
    * `/drives/rootfs`
    * optional `/network-interfaces/eth0` (TAP)
    * optional MMDS (`/mmds/config`, `/mmds`)
3. Starts the instance.
4. Waits for the guest JSON marker (or halt strings).
5. Calls `FlushMetrics` and waits for `metrics.log` to advance.
6. Stops Firecracker and prints a JSON receipt.

---

# Running on Google Cloud

Firecracker requires KVM. On Google Compute Engine that means nested virtualization.

## Create a GCE VM (nested virtualization enabled)

From a local machine with `gcloud` configured:

```bash
ZONE=us-central1-a
NAME=fc-kvm-1
IMAGE=ubuntu-2404-noble-amd64-v20260128

gcloud compute instances create $NAME \
  --zone=$ZONE \
  --machine-type=n2-standard-4 \
  --image-project=ubuntu-os-cloud \
  --image=$IMAGE \
  --boot-disk-size=50GB \
  --boot-disk-type=pd-ssd \
  --enable-nested-virtualization

```

SSH into it:

```bash
gcloud compute ssh "$NAME" --zone="$ZONE"
```

### Verify `/dev/kvm` exists

Inside the VM:

```bash
ls -l /dev/kvm
```

If `/dev/kvm` is missing, nested virtualization is not active or the machine family does not support it.

---

# Host setup (inside the GCE VM)

## Install packages

```bash
sudo apt-get update && sudo apt-get install -y \
  git curl jq unzip build-essential wget \
  squashfs-tools e2fsprogs openssh-client
```

## Install Go (example)

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

## Google Cloud prerequisites

* `gcloud` CLI installed and authenticated
* A GCP project selected: `gcloud config set project <PROJECT_ID>`
* Compute Engine API enabled for the project
* Permissions to create VM instances (at minimum: `compute.instances.create`, `compute.disks.create`, `compute.subnetworks.use`, `compute.instances.setMetadata` is commonly needed)
* A VPC network and subnet available (default VPC is fine)
* Quota for:

    * 1x `n2-standard-4` VM in the chosen zone
    * 50 GB Persistent Disk SSD

Optional but useful:

* Firewall allows SSH (default rule typically exists)
* Local SSH key set up for `gcloud compute ssh` (handled automatically if not)




## VM side prerequisites

After SSHing in, install basics:

```bash
sudo apt-get update
sudo apt-get install -y \
  git curl wget jq unzip build-essential \
  squashfs-tools e2fsprogs openssh-client \
  iproute2
```

Verify KVM is present:

```bash
ls -l /dev/kvm
```

If `/dev/kvm` exists but is not usable as the current user:

```bash
sudo usermod -aG kvm "$USER"
newgrp kvm
sudo chgrp kvm /dev/kvm
sudo chmod g+rw /dev/kvm
ls -l /dev/kvm
```

Install Go (example version used previously):

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

From there, the repo flow stays the same:

```bash
git clone <repo>
cd fc-metrics

make artifacts
make patch
make build
make run
```

For TAP and MMDS testing:

```bash
sudo make run-net
```

---
# Miscellaneous

## Firecracker artifacts

Artifacts are placed in `~/fc-artifacts/`:

* `firecracker` (Firecracker binary)
* `vmlinux` (guest kernel)
* `ubuntu-24.04.ext4` (guest root filesystem)

## Create the artifacts directory

```bash
mkdir -p ~/fc-artifacts
cd ~/fc-artifacts
```

## Download Firecracker binary (GitHub release tarball)

```bash
ARCH="$(uname -m)"
release_url="https://github.com/firecracker-microvm/firecracker/releases"
latest="$(basename "$(curl -fsSLI -o /dev/null -w %{url_effective} ${release_url}/latest)")"

curl -L "${release_url}/download/${latest}/firecracker-${latest}-${ARCH}.tgz" | tar -xz
mv "release-${latest}-${ARCH}/firecracker-${latest}-${ARCH}" ./firecracker
chmod +x ./firecracker
./firecracker --version
```

## Download kernel and Ubuntu rootfs from Firecracker CI bucket

```bash
CI_VERSION="v1.14"
ARCH="x86_64"

# Kernel
KERNEL_KEY=$(
  curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-&list-type=2" \
  | grep -oE "firecracker-ci/${CI_VERSION}/${ARCH}/vmlinux-[0-9]+\.[0-9]+\.[0-9]+" \
  | sort -V \
  | tail -1 \
  | tr -d '\r\n'
)
echo "Kernel key: [$KERNEL_KEY]"
wget -O vmlinux "https://s3.amazonaws.com/spec.ccfc.min/${KERNEL_KEY}"

# Ubuntu squashfs
UBUNTU_KEY=$(
  curl -fsSL "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-&list-type=2" \
  | grep -oE "firecracker-ci/${CI_VERSION}/${ARCH}/ubuntu-[0-9]+\.[0-9]+\.squashfs" \
  | sort -V \
  | tail -1 \
  | tr -d '\r\n'
)
echo "Ubuntu key: [$UBUNTU_KEY]"

ubuntu_version="$(basename "${UBUNTU_KEY}" .squashfs | grep -oE '[0-9]+\.[0-9]+')"
wget -O "ubuntu-${ubuntu_version}.squashfs.upstream" "https://s3.amazonaws.com/spec.ccfc.min/${UBUNTU_KEY}"

rm -rf squashfs-root
unsquashfs "ubuntu-${ubuntu_version}.squashfs.upstream"
```


## Convert squashfs-root to ext4

```bash
truncate -s 1G "ubuntu-${ubuntu_version}.ext4"
sudo mkfs.ext4 -d squashfs-root -F "ubuntu-${ubuntu_version}.ext4"
sudo e2fsck -fn "ubuntu-${ubuntu_version}.ext4" >/dev/null && echo "Rootfs fsck: OK"

# Convenience name used by examples
ln -sf "ubuntu-${ubuntu_version}.ext4" ubuntu-24.04.ext4
```

---

# Guest task wiring (fc-task.service + fc_task.sh)

The guest must do two things:

* emit a single JSON marker line to `/dev/console`
* power off when complete

## Mount the rootfs and edit

```bash
sudo mkdir -p /mnt/fcroot
sudo mount -o loop ~/fc-artifacts/ubuntu-24.04.ext4 /mnt/fcroot
```

Edit the guest task script:

```bash
sudo vim /mnt/fcroot/usr/local/bin/fc_task.sh
sudo chmod +x /mnt/fcroot/usr/local/bin/fc_task.sh
```

Ensure the systemd unit exists and is enabled:

```bash
sudo vim /mnt/fcroot/etc/systemd/system/fc-task.service
sudo chroot /mnt/fcroot systemctl enable fc-task.service >/dev/null 2>&1 || true
```

Unmount cleanly:

```bash
sudo sync
sudo umount /mnt/fcroot
```

---

# Install and run fc-metrics

## Clone

```bash
cd ~
git clone git@github.com:BirdyFoot/fc-metrics.git
cd fc-metrics
```

## Build

```bash
go build ./cmd/fc-run
```

Binary output:

```bash
./cmd/fc-run/fc-run
```

## Run (disk + workspace metrics)

```bash
./cmd/fc-run/fc-run \
  -fc ~/fc-artifacts/firecracker \
  -kernel ~/fc-artifacts/vmlinux \
  -rootfs ~/fc-artifacts/ubuntu-24.04.ext4 \
  -timeout 30s \
  -keep
```

Expected: non-zero `block_read_bytes` and `block_write_bytes`, plus guest workspace deltas when the guest emits the JSON marker.

## Run with networking + MMDS (net byte counters)

TAP creation requires `NET_ADMIN`. Simplest approach is `sudo`:

```bash
sudo ./cmd/fc-run/fc-run \
  -fc ~/fc-artifacts/firecracker \
  -kernel ~/fc-artifacts/vmlinux \
  -rootfs ~/fc-artifacts/ubuntu-24.04.ext4 \
  -timeout 30s \
  -keep \
  -net \
  -mmds
```

Expected: non-zero `net_rx_bytes` and `net_tx_bytes` once a NIC is attached and some traffic exists (MMDS requests are sufficient to produce baseline network activity).

---

# Debugging

With `-keep`, each run writes to `/tmp/run-<id>/`:

* `firecracker.log` (Firecracker stdout/stderr plus guest serial)
* `metrics.log` (Firecracker metrics snapshots)

## Confirm guest marker was emitted

```bash
RUN_DIR=$(ls -td /tmp/run-* | head -n 1)
grep -n "workspace_files_delta" "$RUN_DIR/firecracker.log" | tail -n 5
```

## Inspect metrics snapshots

```bash
wc -l "$RUN_DIR/metrics.log"
tail -n 5 "$RUN_DIR/metrics.log"
```

## Common gotchas

* The correct unmount command is `sudo umount /mnt/fcroot` (not `unmount`).
* If metrics are missing, Firecracker may have been killed before metrics flushed or before the metrics file advanced.
* Network bytes remain zero unless:

    * a NIC is attached via Firecracker API
    * a TAP device exists and is up
    * some traffic occurs inside the guest (MMDS fetch is a simple source of traffic)

---
