package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"fc-metrics/pkg/fcrun"
	"fc-metrics/pkg/weather"
)

func main() {
	var (
		fcBin  = flag.String("fc", "", "Path to firecracker binary")
		kernel = flag.String("kernel", "", "Path to guest kernel image (vmlinux)")
		rootfs = flag.String("rootfs", "", "Path to guest rootfs ext4 image")
		lat    = flag.Float64("lat", 41.8781, "Latitude")
		lon    = flag.Float64("lon", -87.6298, "Longitude")
		keep   = flag.Bool("keep", false, "Keep /tmp/run-* directory")
	)
	flag.Parse()

	if *fcBin == "" || *kernel == "" || *rootfs == "" {
		fmt.Fprintln(os.Stderr, "ERROR: required flags: -fc -kernel -rootfs")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cur, err := weather.FetchCurrent(ctx, *lat, *lon)
	if err != nil {
		fmt.Fprintln(os.Stderr, "weather fetch failed:", err)
		os.Exit(1)
	}

	mmdsData := map[string]any{
		"latest": map[string]any{
			"meta-data": map[string]any{
				"weather_current": cur,
			},
		},
	}

	r := fcrun.New()
	receipt, err := r.Run(ctx, fcrun.RunConfig{
		FirecrackerBin: *fcBin,
		KernelImage:    *kernel,
		RootFS:         *rootfs,
		VCPUs:          2,
		MemMiB:         2048,
		TimeoutSeconds: 30,
		KeepRunDir:     *keep,

		Network: fcrun.NetworkConfig{
			EnableTap: true,
			IfaceID:   "eth0",
			GuestMAC:  "02:FC:00:00:00:01",
		},
		MMDS: fcrun.MMDSConfig{
			Enable:  true,
			Version: "V2",
			Data:    mmdsData,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "run failed:", err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(receipt, "", "  ")
	fmt.Println(string(out))
}

