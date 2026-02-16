package fcrun

import (
	"bufio"
	"encoding/json"
	"strings"
)

func ParseFirecrackerMetrics1141(raw string) (netRx, netTx, blkRead, blkWrite uint64, lines int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, 0, 0, 0
	}

	getU64 := func(m map[string]any, k string) (uint64, bool) {
		v, ok := m[k]
		if !ok {
			return 0, false
		}
		switch t := v.(type) {
		case json.Number:
			i, err := t.Int64()
			if err != nil || i < 0 {
				return 0, false
			}
			return uint64(i), true
		case float64:
			if t < 0 {
				return 0, false
			}
			return uint64(t), true
		case uint64:
			return t, true
		default:
			return 0, false
		}
	}

	sc := bufio.NewScanner(strings.NewReader(raw))
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4*1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		lines++

		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()

		var obj map[string]any
		if dec.Decode(&obj) != nil {
			continue
		}

		var netKeys []string
		var blkKeys []string

		for k := range obj {
			if strings.HasPrefix(k, "net_") && k != "net" {
				netKeys = append(netKeys, k)
			}
			if strings.HasPrefix(k, "block_") && k != "block" {
				blkKeys = append(blkKeys, k)
			}
		}

		if len(netKeys) == 0 {
			if _, ok := obj["net"]; ok {
				netKeys = []string{"net"}
			}
		}
		if len(blkKeys) == 0 {
			if _, ok := obj["block"]; ok {
				blkKeys = []string{"block"}
			}
		}

		for _, k := range netKeys {
			m, ok := obj[k].(map[string]any)
			if !ok {
				continue
			}
			if v, ok := getU64(m, "rx_bytes_count"); ok {
				netRx += v
			}
			if v, ok := getU64(m, "tx_bytes_count"); ok {
				netTx += v
			}
		}

		for _, k := range blkKeys {
			m, ok := obj[k].(map[string]any)
			if !ok {
				continue
			}
			if v, ok := getU64(m, "read_bytes"); ok {
				blkRead += v
			}
			if v, ok := getU64(m, "write_bytes"); ok {
				blkWrite += v
			}
		}
	}

	return netRx, netTx, blkRead, blkWrite, lines
}

