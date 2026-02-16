package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Current struct {
	Time        string  `json:"time"`
	Temperature float64 `json:"temperature_2m"`
	WeatherCode int     `json:"weather_code"`
}

type openMeteoResp struct {
	Current Current `json:"current"`
}

func FetchCurrent(ctx context.Context, lat, lon float64) (Current, error) {
	// Open-Meteo docs: /v1/forecast with required latitude and longitude, and supports "current" variables. :contentReference[oaicite:2]{index=2}
	u := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.6f&longitude=%.6f&current=temperature_2m,weather_code&timezone=UTC", lat, lon)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Current{}, err
	}

	c := &http.Client{Timeout: 8 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return Current{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Current{}, fmt.Errorf("open-meteo http %s", resp.Status)
	}

	var out openMeteoResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Current{}, err
	}
	return out.Current, nil
}

