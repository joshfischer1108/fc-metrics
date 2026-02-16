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
	)
	flag.Parse()

	if *fcBin == "" || *kernel == "" || *rootfs == "" {
		fmt.Fprintln(os.Stderr, "ERROR: required flags: -fc -kernel -rootfs")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	r := fcrun.New()
	receipt, err := r.Run(ctx, fcrun.RunConfig{
		FirecrackerBin:    *fcBin,
		KernelImage:       *kernel,
		RootFS:            *rootfs,
		VCPUs:             *vcpus,
		MemMiB:            *memMiB,
		TimeoutSeconds:    int(timeout.Seconds()),
		IncludeRawMetrics: *raw,
		KeepRunDir:        *keep,

		// No NIC/MMDS by default for this simple runner.
		Network: fcrun.NetworkConfig{EnableTap: false},
		MMDS:    fcrun.MMDSConfig{Enable: false},
	})
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

