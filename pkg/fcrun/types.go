package fcrun

import "time"

type Receipt struct {
	RunID      string    `json:"run_id"`
	StartedAt  time.Time `json:"started_at"`
	EndedAt    time.Time `json:"ended_at"`
	DurationMs int64     `json:"duration_ms"`
	ExitCode   int       `json:"exit_code"`

	Kernel string `json:"kernel"`
	Rootfs string `json:"rootfs"`
	FCBin  string `json:"firecracker_bin"`

	VCPUs  int `json:"vcpus"`
	MemMiB int `json:"mem_mib"`

	NetRxBytes   uint64 `json:"net_rx_bytes"`
	NetTxBytes   uint64 `json:"net_tx_bytes"`
	BlockReadB   uint64 `json:"block_read_bytes"`
	BlockWriteB  uint64 `json:"block_write_bytes"`
	MetricsLines int    `json:"metrics_lines"`

	WorkspaceFilesDelta int64 `json:"workspace_files_delta,omitempty"`
	WorkspaceBytesDelta int64 `json:"workspace_bytes_delta,omitempty"`

	WaitErr string `json:"wait_err,omitempty"`

	MetricsRaw string `json:"metrics_raw,omitempty"`
	LogPath    string `json:"firecracker_log_path,omitempty"`
}

type NetworkConfig struct {
	EnableTap bool
	IfaceID   string
	TapName   string
	GuestMAC  string
}

type MMDSConfig struct {
	Enable  bool
	Version string // "V2"
	Data    map[string]any
}

type RunConfig struct {
	FirecrackerBin string
	KernelImage    string
	RootFS         string

	VCPUs  int
	MemMiB int

	TimeoutSeconds int

	// guest done detection
	MarkerPrefix  string
	MarkerJSONKey string

	// optional
	IncludeRawMetrics bool
	KeepRunDir        bool

	Network NetworkConfig
	MMDS    MMDSConfig
}
