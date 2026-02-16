package fcrun

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func unixHTTPClient(sock string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sock)
			},
		},
		Timeout: 10 * time.Second,
	}
}

func putJSON(ctx context.Context, c *http.Client, path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://unix"+path, strings.NewReader(string(b)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := readSmall(resp.Body, 4096)
		return fmt.Errorf("PUT %s failed: %s: %s", path, resp.Status, msg)
	}
	return nil
}

func readSmall(r io.Reader, max int) string {
	br := bufio.NewReader(r)
	b, _ := br.Peek(max)
	return string(b)
}

func waitForUnixSocket(ctx context.Context, path string, max time.Duration) error {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, err := os.Stat(path); err == nil {
			c, err := net.DialTimeout("unix", path, 200*time.Millisecond)
			if err == nil {
				_ = c.Close()
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for unix socket %s", path)
}

func waitForMetricsAdvance(ctx context.Context, metricsPath string, max time.Duration) error {
	deadline := time.Now().Add(max)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var size0 int64
	var mt0 time.Time
	seen := false

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			st, err := os.Stat(metricsPath)
			if err != nil {
				continue
			}
			if !seen {
				seen = true
				size0 = st.Size()
				mt0 = st.ModTime()
				continue
			}
			if st.Size() > size0 || st.ModTime().After(mt0) {
				return nil
			}
		}
	}
	return fmt.Errorf("metrics did not advance within %s", max)
}

