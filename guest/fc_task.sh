#!/bin/bash
set -euo pipefail

# Bring up NIC (no IP needed for this MMDS-only test, but link must be up)
ip link set eth0 up 2>/dev/null || true

# Some setups need an explicit route for the MMDS IP
ip route add 169.254.169.254/32 dev eth0 2>/dev/null || true

MMDS_IP="169.254.169.254"
IFACE="eth0"

ip link set "$IFACE" up 2>/dev/null || true
ip route add ${MMDS_IP}/32 dev "$IFACE" 2>/dev/null || true

TOKEN="$(curl -sS --connect-timeout 1 -m 2 \
  -X PUT "http://${MMDS_IP}/latest/api/token" \
  -H "X-metadata-token-ttl-seconds: 60" 2>/dev/null || true)"

if [ -n "$TOKEN" ]; then
  curl -sS --connect-timeout 1 -m 2 \
    -H "X-metadata-token: ${TOKEN}" \
    "http://${MMDS_IP}/latest/meta-data/run_id" >/dev/null 2>&1 || true
  echo "MMDS_FETCH=ok" > /dev/console
else
  echo "MMDS_FETCH=token_failed" > /dev/console
fi

STAMP="fc_task_v4_$(date -u +%s)"
echo "STAMP=$STAMP" > /dev/console

BASE="/workspace"
WORKDIR="${BASE}/run-${STAMP}"

# Hard fail if anything is empty
if [ -z "${BASE}" ] || [ -z "${WORKDIR}" ]; then
  echo "ERROR: BASE or WORKDIR empty (BASE='${BASE}' WORKDIR='${WORKDIR}')" > /dev/console
  poweroff -f
  exit 1
fi

mkdir -p "${WORKDIR}"

count_files() { find "$1" -type f 2>/dev/null | wc -l | tr -d ' '; }
count_bytes() { du -sb "$1" 2>/dev/null | awk '{print $1}' | tr -d ' '; }

before_files=$(count_files "${WORKDIR}"); [ -z "$before_files" ] && before_files=0
before_bytes=$(count_bytes "${WORKDIR}"); [ -z "$before_bytes" ] && before_bytes=0
echo "before_files=$before_files before_bytes=$before_bytes" > /dev/console

echo "hello" > "${WORKDIR}/out.txt"
echo "world" > "${WORKDIR}/out2.txt"
dd if=/dev/zero of="${WORKDIR}/blob.bin" bs=1M count=5 status=none
sync

after_files=$(count_files "${WORKDIR}"); [ -z "$after_files" ] && after_files=0
after_bytes=$(count_bytes "${WORKDIR}"); [ -z "$after_bytes" ] && after_bytes=0
echo "after_files=$after_files after_bytes=$after_bytes" > /dev/console

delta_files=$((after_files - before_files))
delta_bytes=$((after_bytes - before_bytes))

echo "{\"workspace_files_delta\":${delta_files},\"workspace_bytes_delta\":${delta_bytes},\"stamp\":\"$STAMP\"}" > /dev/console
poweroff -f
