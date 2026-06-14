// Package climatemeteo is the library behind the climatemeteo command line:
// the HTTP client, request shaping, and the typed data models for the
// Open-Meteo Climate Change API.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package climatemeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// DefaultUserAgent identifies the client to Open-Meteo. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "climatemeteo/dev (+https://github.com/tamnd/climatemeteo-cli)"

// Host is the climate API host this client talks to.
const Host = "climate-api.open-meteo.com"

// Config holds the tunables for a Client.
type Config struct {
	BaseURL string
	Rate    time.Duration
	Retries int
	Timeout time.Duration
}

// DefaultConfig returns sensible defaults for the Open-Meteo Climate API.
func DefaultConfig() Config {
	return Config{
		BaseURL: "https://climate-api.open-meteo.com",
		Rate:    0,
		Retries: 3,
		Timeout: 30 * time.Second,
	}
}

// Client talks to the Open-Meteo Climate Change API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	BaseURL   string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: DefaultUserAgent,
		BaseURL:   cfg.BaseURL,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings. The caller owns nothing extra; the body is read
// fully and closed here.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// ClimateDailyForecast is one day of climate projection data.
type ClimateDailyForecast struct {
	Time            string  `kit:"id" json:"time"`
	TemperatureMean float64 `json:"temperature_mean"`
	PrecipSum       float64 `json:"precip_sum"`
}

// wireClimateResponse is the raw JSON shape returned by the climate API.
type wireClimateResponse struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Daily     struct {
		Time            []string  `json:"time"`
		TemperatureMean []float64 `json:"temperature_2m_mean"`
		PrecipSum       []float64 `json:"precipitation_sum"`
	} `json:"daily"`
}

// DailyParams holds the parameters for a daily climate projection request.
type DailyParams struct {
	Lat   float64
	Lon   float64
	Start string
	End   string
	Model string
}

// Daily fetches daily climate projections for the given parameters.
func (c *Client) Daily(ctx context.Context, p DailyParams) ([]*ClimateDailyForecast, error) {
	if p.Start == "" {
		p.Start = "2030-01-01"
	}
	if p.End == "" {
		p.End = "2030-01-07"
	}
	if p.Model == "" {
		p.Model = "EC_Earth3P_HR"
	}

	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(p.Lat, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(p.Lon, 'f', -1, 64))
	q.Set("start_date", p.Start)
	q.Set("end_date", p.End)
	q.Set("daily", "temperature_2m_mean,precipitation_sum")
	q.Set("models", p.Model)

	rawURL := c.BaseURL + "/v1/climate?" + q.Encode()
	body, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	var wire wireClimateResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	n := len(wire.Daily.Time)
	out := make([]*ClimateDailyForecast, n)
	for i := 0; i < n; i++ {
		row := &ClimateDailyForecast{Time: wire.Daily.Time[i]}
		if i < len(wire.Daily.TemperatureMean) {
			row.TemperatureMean = wire.Daily.TemperatureMean[i]
		}
		if i < len(wire.Daily.PrecipSum) {
			row.PrecipSum = wire.Daily.PrecipSum[i]
		}
		out[i] = row
	}
	return out, nil
}
