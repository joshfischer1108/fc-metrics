package fcrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type Runner struct{}

func New() *Runner { return &Runner{} }

func (r *Runner) Run(ctx context.Context, cfg RunConfig) (Receipt, error) {
	if cfg.FirecrackerBin == "" || cfg.KernelImage == "" || cfg.RootFS == "" {
		return Receipt{}, fmt.Errorf("missing required config: FirecrackerBin, KernelImage, RootFS")
	}
	if cfg.VCPUs <= 0 {
		cfg.VCPUs = 2
	}
	if cfg.MemMiB <= 0 {
		cfg.MemMiB = 2048
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 120
	}
	if cfg.MarkerJSONKey == "" {
		cfg.MarkerJSONKey = "workspace_files_delta"
	}

	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	runDir := filepath.Join(os.TempDir(), runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return Receipt{}, err
	}
	cleanup := func() {
		if !cfg.KeepRunDir {
			_ = os.RemoveAll(runDir)
		}
	}

	apiSock := filepath.Join(runDir, "fc.sock")
	metricsPath := filepath.Join(runDir, "metrics.log")
	logPath := filepath.Join(runDir, "firecracker.log")

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	cmd := exec.Command(cfg.FirecrackerBin, "--api-sock", apiSock)

	logFile, err := os.Create(logPath)
	if err != nil {
		cleanup()
		return Receipt{}, err
	}
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		cleanup()
		return Receipt{}, err
	}

	// best-effort cleanup if anything goes wrong
	defer func() {
		_ = stopProcess(cmd, 500*time.Millisecond)
		_ = cmd.Wait()
		cleanup()
	}()

	if err := waitForUnixSocket(timeoutCtx, apiSock, 5*time.Second); err != nil {
		return Receipt{}, err
	}
	client := unixHTTPClient(apiSock)

	// optional networking (tap + nic attach)
	if cfg.Network.EnableTap {
		if cfg.Network.IfaceID == "" {
			cfg.Network.IfaceID = "eth0"
		}
		if cfg.Network.TapName == "" {
			cfg.Network.TapName = genTapName()
		}
		if cfg.Network.GuestMAC == "" {
			cfg.Network.GuestMAC = "02:FC:00:00:00:01"
		}
		if err := createTap(cfg.Network.TapName); err != nil {
			return Receipt{}, err
		}
		defer deleteTap(cfg.Network.TapName)

		// Firecracker requires iface_id and host_dev_name. :contentReference[oaicite:1]{index=1}
		if err := putJSON(timeoutCtx, client, "/network-interfaces/"+cfg.Network.IfaceID, map[string]any{
			"iface_id":      cfg.Network.IfaceID,
			"host_dev_name": cfg.Network.TapName,
			"guest_mac":     cfg.Network.GuestMAC,
		}); err != nil {
			return Receipt{}, err
		}
	}

	// metrics first
	if err := putJSON(timeoutCtx, client, "/metrics", map[string]any{
		"metrics_path":              metricsPath,
		"metrics_flush_interval_ms": 200,
	}); err != nil {
		return Receipt{}, err
	}

	if err := putJSON(timeoutCtx, client, "/machine-config", map[string]any{
		"vcpu_count":   cfg.VCPUs,
		"mem_size_mib": cfg.MemMiB,
		"smt":          false,
	}); err != nil {
		return Receipt{}, err
	}

	bootArgs := "console=ttyS0 reboot=k panic=1 pci=off systemd.unit=fc-task.service"
	if err := putJSON(timeoutCtx, client, "/boot-source", map[string]any{
		"kernel_image_path": cfg.KernelImage,
		"boot_args":         bootArgs,
	}); err != nil {
		return Receipt{}, err
	}

	// Create a per-run copy of the rootfs so runs are isolated and repeatable.
	rootfsCopy := filepath.Join(runDir, "rootfs.ext4")
	if err := copyFile(cfg.RootFS, rootfsCopy); err != nil {
		return Receipt{}, fmt.Errorf("copy rootfs: %w", err)
	}

	// attach the copied rootfs to the VM.
	if err := putJSON(timeoutCtx, client, "/drives/rootfs", map[string]any{
		"drive_id":       "rootfs",
		"path_on_host":   rootfsCopy,
		"is_root_device": true,
		"is_read_only":   false,
	}); err != nil {
		return Receipt{}, err
	}

	// optional MMDS V2 config and data
	if cfg.MMDS.Enable {
		version := cfg.MMDS.Version
		if version == "" {
			version = "V2"
		}
		if err := putJSON(timeoutCtx, client, "/mmds/config", map[string]any{
			"network_interfaces": []string{"eth0"},
			"version":            version,
		}); err != nil {
			return Receipt{}, err
		}
		if cfg.MMDS.Data != nil {
			if err := putJSON(timeoutCtx, client, "/mmds", cfg.MMDS.Data); err != nil {
				return Receipt{}, err
			}
		}
	}

	started := time.Now()
	if err := putJSON(timeoutCtx, client, "/actions", map[string]any{"action_type": "InstanceStart"}); err != nil {
		return Receipt{}, err
	}

	done, waitErr := waitForGuestDone(timeoutCtx, logPath, cfg.MarkerPrefix, cfg.MarkerJSONKey)

	exitCode := 0
	waitErrStr := ""
	if waitErr != nil {
		exitCode = 124
		waitErrStr = waitErr.Error()
	} else if !done.SeenMarker {
		exitCode = 124
		waitErrStr = "guest completion marker not observed"
	}

	if exitCode == 0 {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = putJSON(flushCtx, client, "/actions", map[string]any{"action_type": "FlushMetrics"})
		flushCancel()

		advanceCtx, advanceCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = waitForMetricsAdvance(advanceCtx, metricsPath, 2*time.Second)
		advanceCancel()
	}

	_ = stopProcess(cmd, 2*time.Second)
	_ = cmd.Wait()
	ended := time.Now()

	metricsRawBytes, _ := os.ReadFile(metricsPath)
	metricsRaw := string(metricsRawBytes)
	netRx, netTx, blkR, blkW, lines := ParseFirecrackerMetrics1141(metricsRaw)

	receipt := Receipt{
		RunID:        runID,
		StartedAt:    started,
		EndedAt:      ended,
		DurationMs:   ended.Sub(started).Milliseconds(),
		ExitCode:     exitCode,
		Kernel:       cfg.KernelImage,
		Rootfs:       cfg.RootFS,
		FCBin:        cfg.FirecrackerBin,
		VCPUs:        cfg.VCPUs,
		MemMiB:       cfg.MemMiB,
		NetRxBytes:   netRx,
		NetTxBytes:   netTx,
		BlockReadB:   blkR,
		BlockWriteB:  blkW,
		MetricsLines: lines,
		LogPath:      logPath,
		WaitErr:      waitErrStr,
	}

	if done.SeenMarker {
		receipt.WorkspaceFilesDelta = done.FilesDelta
		receipt.WorkspaceBytesDelta = done.BytesDelta
	}
	if cfg.IncludeRawMetrics {
		receipt.MetricsRaw = metricsRaw
	}

	// on success, do normal cleanup decision
	if !cfg.KeepRunDir {
		_ = os.RemoveAll(runDir)
	}
	return receipt, nil
}

func genTapName() string {
	b := make([]byte, 5) // 10 hex chars
	_, _ = rand.Read(b)
	return "tap" + hex.EncodeToString(b) // "tap" + 10 = 13 chars
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	st, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
