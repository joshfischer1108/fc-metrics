# fc-metrics

Host-side Firecracker runner that emits a single JSON receipt per microVM run. The receipt includes disk counters, optional network counters, and guest-produced “workspace delta” measurements.

## What it is

`fc-metrics` coordinates a Firecracker microVM run and prints one JSON object at the end:

* boots a microVM
* waits for a completion marker from the guest (serial console)
* forces Firecracker to flush metrics
* parses metrics and prints a single per-run receipt

Guest behavior lives inside the root filesystem image. The host runner stays intentionally thin.

## What problem it solves

Firecracker lifecycle and metrics collection are easy to get subtly wrong:

* A guest can halt while Firecracker keeps running, so relying on Firecracker process exit is brittle.
* Metrics can be incomplete unless `FlushMetrics` is called and the metrics file is allowed to advance. 
* A clean “done” signal from inside the guest is needed to mark completion reliably.
* Producing one portable record per run is simpler than scraping logs.

`fc-metrics` provides a repeatable pattern for producing one JSON receipt per run.

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

## Concepts

### squashfs vs ext4 rootfs

* `ubuntu-*.squashfs.upstream` is a compressed, read-only filesystem image that is easy to distribute.
* `ubuntu-*.ext4` is a writable disk image used as the VM root disk. It is created by unpacking squashfs and building an ext4 filesystem so the guest can write files and systemd units can be installed/modified.

### TAP devices

A TAP device is a host-side virtual NIC. Firecracker can attach a TAP as a guest NIC, which enables network byte counters and MMDS traffic generation.

### MMDS

MMDS is Firecracker’s metadata service, configured through the Firecracker API. 
Firecracker’s device model intercepts ARP and TCP packets destined for the MMDS IP (default `169.254.169.254`) and serves metadata responses without sending that traffic to the TAP device. 

## Host requirements

* Linux host with KVM (`/dev/kvm`)
* x86_64 host is the simplest path for the provided artifacts
* Firecracker requires KVM access; for cloud hosts this typically means nested virtualization

On Google Compute Engine, nested virtualization is not supported on Arm CPUs (and is also not supported on AMD processors for this feature), so Apple Silicon workflows typically use a remote x86_64 Linux host. ([Google Cloud Documentation][1])

---

# Quick start (Google Cloud)

Firecracker needs KVM. On GCE that means nested virtualization.

## Prerequisites (local machine)

* `gcloud` CLI installed and authenticated
* project selected: `gcloud config set project <PROJECT_ID>`
* Compute Engine API enabled
* permissions to create instances
* quota for the chosen machine type and disk

## Create a GCE VM with nested virtualization

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

Wait 30–60 seconds, then SSH in:

```bash
gcloud compute ssh "$NAME" --zone="$ZONE"
```

Verify KVM exists:

```bash
ls -l /dev/kvm
```

If `/dev/kvm` is missing, nested virtualization is not active or the instance type does not support it.

## Allow non-root access to /dev/kvm

The common failure mode is:

`Permission denied (os error 13) ... Make sure the user launching the firecracker process is configured on the /dev/kvm file's ACL.`

Fix by adding the user to the `kvm` group:

```bash
sudo usermod -aG kvm "$USER"
```

Make the group change take effect;

```bash
exit
gcloud compute ssh "$NAME" --zone="$ZONE"
```

Confirm membership:

```bash
id | grep -Eo 'kvm' || true
ls -l /dev/kvm
```

Optional: apply group permissions (usually already correct on Ubuntu images):

```bash
sudo chgrp kvm /dev/kvm
sudo chmod g+rw /dev/kvm
ls -l /dev/kvm
```

---

# Install dependencies (inside the GCE VM)

```bash
sudo apt-get update && sudo apt-get install -y \
  git curl jq unzip build-essential wget \
  squashfs-tools e2fsprogs openssh-client iproute2
```

Install Go:

```bash
cd /tmp
curl -LO https://go.dev/dl/go1.22.2.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.22.2.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

---

# Build and run

```bash
cd ~/
git clone https://github.com/joshfischer1108/fc-metrics.git
cd fc-metrics

make artifacts
make patch
make build
make run
```

After successful completion, the receipt JSON is printed to stdout.
e.g. 
```json
{
  "run_id": "run-…",
  "started_at": "2026-02-17T17:20:46Z",
  "ended_at": "2026-02-17T17:20:53Z",
  "duration_ms": 6594,
  "exit_code": 0,
  "vcpus": 2,
  "mem_mib": 2048,
  "net_rx_bytes": 0,
  "net_tx_bytes": 0,
  "block_read_bytes": 51203072,
  "block_write_bytes": 6451200,
  "workspace_files_delta": 3,
  "workspace_bytes_delta": 5242892
}
```

For TAP and MMDS testing:

```bash
make run-net
```

---

# What just happened

## `make artifacts`

Downloads or assembles runtime artifacts into `~/fc-artifacts/`:

* Firecracker binary
* guest kernel (`vmlinux`)
* upstream Ubuntu squashfs
* writable Ubuntu ext4 (root disk used by the microVM)
* optional SSH key material used during image creation

## `make patch`

Applies guest-side wiring into the ext4 image:

* installs `/usr/local/bin/fc_task.sh`
* installs/enables `fc-task.service` (systemd oneshot)

The guest task is responsible for two things:

1. write exactly one JSON “done marker” line to `/dev/console`
2. power off

That marker is parsed from the serial console by the host runner.

## `make build`

Builds the host-side runner (`fc-run`) and demos.

## `make run`

Runs one microVM and prints a receipt JSON. Disk counters come from Firecracker metrics and are flushed at the end using the `FlushMetrics` action.
Metrics output is written to the configured `metrics_path`. 

## `make run-net`

Same as `make run`, plus:

* creates a host TAP device and attaches it as `eth0` in the microVM
* enables MMDS on `eth0`
* causes network activity (MMDS fetches are sufficient), so `net_rx_bytes` and `net_tx_bytes` become non-zero

TAP creation requires `CAP_NET_ADMIN`, hence `sudo`.

---

# Guest assets in the repo

Guest content should be stored as versioned files in the repository and copied into the ext4 image by `make patch`. A simple layout:

```
guest/
  fc_task.sh
  fc-task.service
```

`make patch` becomes the single source of truth for what goes into the guest image, instead of editing a mounted image by hand.

---

# Debugging

With `-keep` (or the Makefile equivalent), each run writes to `/tmp/run-<id>/`:

* `firecracker.log` (Firecracker logs plus guest serial)
* `metrics.log` (Firecracker metrics snapshots)

Confirm the guest marker was emitted:

```bash
RUN_DIR=$(ls -td /tmp/run-* | head -n 1)
grep -n "workspace_files_delta" "$RUN_DIR/firecracker.log" | tail -n 5
```

Inspect metrics snapshots:

```bash
wc -l "$RUN_DIR/metrics.log"
tail -n 5 "$RUN_DIR/metrics.log"
```

Common gotchas:

* correct unmount command is `sudo umount /mnt/fcroot` (not `unmount`)
* network bytes stay at zero unless a NIC is attached and traffic happens
* TAP + MMDS requires root privileges (or `cap_net_admin`)

---
