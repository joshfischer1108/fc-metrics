package fcrun

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"
)

type GuestDone struct {
	FilesDelta int64
	BytesDelta int64
	SeenMarker bool
}

func waitForGuestDone(ctx context.Context, logPath string, markerPrefix string, markerJSONKey string) (GuestDone, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return GuestDone{}, err
	}
	defer f.Close()

	// follow file as it grows
	_, _ = f.Seek(0, 2)
	r := bufio.NewReader(f)

	for {
		select {
		case <-ctx.Done():
			return GuestDone{}, ctx.Err()
		default:
		}

		line, err := r.ReadString('\n')
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		s := strings.TrimSpace(line)
		if s == "" {
			continue
		}

		if strings.Contains(s, "System halted") || strings.Contains(s, "Powering off.") {
			return GuestDone{SeenMarker: true}, nil
		}

		if markerPrefix != "" {
			if !strings.HasPrefix(s, markerPrefix) {
				continue
			}
			s = strings.TrimSpace(strings.TrimPrefix(s, markerPrefix))
		} else {
			if !strings.HasPrefix(s, "{") {
				continue
			}
		}

		if markerJSONKey != "" && !strings.Contains(s, markerJSONKey) {
			continue
		}

		var m map[string]any
		if json.Unmarshal([]byte(s), &m) != nil {
			continue
		}
		fd, fdok := m["workspace_files_delta"].(float64)
		bd, bdok := m["workspace_bytes_delta"].(float64)
		if fdok && bdok {
			return GuestDone{FilesDelta: int64(fd), BytesDelta: int64(bd), SeenMarker: true}, nil
		}
	}
}

