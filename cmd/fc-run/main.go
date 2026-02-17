package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"fc-metrics/pkg/fcrun"
)

func main() {
	var (
		fcBin   = flag.String("fc", "", "Path to firecracker binary")
		kernel  = flag.String("kernel", "", "Path to guest kernel image (vmlinux)")
		rootfs  = flag.String("rootfs", "", "Path to guest rootfs ext4 image")
		vcpus   = flag.Int("vcpus", 2, "Number of vCPUs")
		memMiB  = flag.Int("mem", 2048, "Memory in MiB")
		timeout = flag.Duration("timeout", 2*time.Minute, "Hard timeout for the microVM run")
		raw     = flag.Bool("raw", false, "Include raw metrics in receipt JSON")
		keep    = flag.Bool("keep", false, "Keep /tmp/run-* directory for debugging")

		netOn  = flag.Bool("net", false, "Attach a TAP-backed NIC (requires sudo/CAP_NET_ADMIN)")
		mmdsOn = flag.Bool("mmds", false, "Enable MMDS v2 (requires -net)")
	)
	flag.Parse()

	if *fcBin == "" || *kernel == "" || *rootfs == "" {
		fmt.Fprintln(os.Stderr, "ERROR: required flags: -fc -kernel -rootfs")
		os.Exit(2)
	}
	if *mmdsOn && !*netOn {
		fmt.Fprintln(os.Stderr, "ERROR: -mmds requires -net (MMDS needs a NIC)")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	cfg := fcrun.RunConfig{
		FirecrackerBin:    *fcBin,
		KernelImage:       *kernel,
		RootFS:            *rootfs,
		VCPUs:             *vcpus,
		MemMiB:            *memMiB,
		TimeoutSeconds:    int((*timeout).Seconds()), // FIX: use *timeout
		IncludeRawMetrics: *raw,
		KeepRunDir:        *keep,
		Network: fcrun.NetworkConfig{
			EnableTap: *netOn,
			IfaceID:   "eth0",
			GuestMAC:  "02:FC:00:00:00:01",
		},
		MMDS: fcrun.MMDSConfig{
			Enable:  *mmdsOn,
			Version: "V2",
			Data: map[string]any{
				"latest": map[string]any{
					"meta-data": map[string]any{
						// guest can read this via MMDS
						"run_id": "placeholder",
					},
				},
			},
		},
	}

	r := fcrun.New()
	receipt, err := r.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "run failed:", err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(receipt, "", "  ")
	fmt.Println(string(out))

	if receipt.ExitCode != 0 {
		os.Exit(receipt.ExitCode)
	}
}
