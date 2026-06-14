package climatemeteo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestDaily(t *testing.T) {
	resp := wireClimateResponse{}
	resp.Latitude = 52.52
	resp.Longitude = 13.41
	resp.Daily.Time = []string{"2030-01-01", "2030-01-02", "2030-01-03"}
	resp.Daily.TemperatureMean = []float64{5.1, 3.2, 4.7}
	resp.Daily.PrecipSum = []float64{0.0, 1.5, 2.2}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/climate" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	rows, err := c.Daily(context.Background(), DailyParams{
		Lat:   52.52,
		Lon:   13.41,
		Start: "2030-01-01",
		End:   "2030-01-03",
		Model: "EC_Earth3P_HR",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if rows[0].Time != "2030-01-01" {
		t.Errorf("rows[0].Time = %q, want 2030-01-01", rows[0].Time)
	}
	if rows[1].TemperatureMean != 3.2 {
		t.Errorf("rows[1].TemperatureMean = %v, want 3.2", rows[1].TemperatureMean)
	}
	if rows[2].PrecipSum != 2.2 {
		t.Errorf("rows[2].PrecipSum = %v, want 2.2", rows[2].PrecipSum)
	}
}

func TestDailyDefaultDates(t *testing.T) {
	var gotStart, gotEnd string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStart = r.URL.Query().Get("start_date")
		gotEnd = r.URL.Query().Get("end_date")
		resp := wireClimateResponse{}
		resp.Daily.Time = []string{"2030-01-01"}
		resp.Daily.TemperatureMean = []float64{5.0}
		resp.Daily.PrecipSum = []float64{0.0}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.BaseURL = srv.URL

	_, err := c.Daily(context.Background(), DailyParams{Lat: 52.52, Lon: 13.41})
	if err != nil {
		t.Fatal(err)
	}
	if gotStart != "2030-01-01" {
		t.Errorf("default start_date = %q, want 2030-01-01", gotStart)
	}
	if gotEnd != "2030-01-07" {
		t.Errorf("default end_date = %q, want 2030-01-07", gotEnd)
	}
}
