package climatemeteo

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes climatemeteo as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/climatemeteo-cli/climatemeteo"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// climatemeteo:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone climatemeteo binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the climatemeteo driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// dateRE matches YYYY-MM-DD strings.
var dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "climatemeteo",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "climatemeteo",
			Short:  "Climate projections from Open-Meteo, no API key required.",
			Long: `climatemeteo reads long-range climate projection data from the
Open-Meteo Climate Change API and prints clean records that pipe
into the rest of your tools. No API key, nothing to run alongside it.`,
			Site: "https://" + Host,
			Repo: "https://github.com/tamnd/climatemeteo-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// daily: list climate projection records per day.
	kit.Handle(app, kit.OpMeta{
		Name:    "daily",
		Group:   "read",
		List:    true,
		Summary: "Daily climate projections for a location and date range",
		URIType: "latlon",
		Args:    []kit.Arg{{Name: "ref", Help: "lat,lon or URL", Optional: true}},
	}, listDaily)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// dailyInput is the flag set for the daily op.
type dailyInput struct {
	Ref    string  `kit:"arg"                  help:"lat,lon coordinates (optional)"`
	Lat    float64 `kit:"flag"                 help:"latitude"`
	Lon    float64 `kit:"flag"                 help:"longitude"`
	Start  string  `kit:"flag"                 help:"start date (YYYY-MM-DD)"`
	End    string  `kit:"flag"                 help:"end date (YYYY-MM-DD)"`
	Model  string  `kit:"flag"                 help:"climate model (default EC_Earth3P_HR)"`
	Limit  int     `kit:"flag,inherit"         help:"max results"`
	Client *Client `kit:"inject"`
}

func listDaily(ctx context.Context, in dailyInput, emit func(*ClimateDailyForecast) error) error {
	lat, lon := in.Lat, in.Lon

	// If a positional ref was given, parse it as lat,lon.
	if in.Ref != "" {
		l, o, err := parseLatLon(in.Ref)
		if err != nil {
			return errs.Usage("ref must be lat,lon: %v", err)
		}
		lat, lon = l, o
	}

	model := in.Model
	if model == "" {
		model = "EC_Earth3P_HR"
	}

	rows, err := in.Client.Daily(ctx, DailyParams{
		Lat:   lat,
		Lon:   lon,
		Start: in.Start,
		End:   in.End,
		Model: model,
	})
	if err != nil {
		return err
	}

	for i, row := range rows {
		if in.Limit > 0 && i >= in.Limit {
			break
		}
		if err := emit(row); err != nil {
			return err
		}
	}
	return nil
}

// Classify turns any accepted input into the canonical (type, id).
//
//   - Two floats separated by comma → ("latlon", input)
//   - Matches YYYY-MM-DD           → ("date", input)
//   - else                          → ("query", input)
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("empty input")
	}
	// latlon: two comma-separated floats
	parts := strings.SplitN(input, ",", 2)
	if len(parts) == 2 {
		_, e1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		_, e2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if e1 == nil && e2 == nil {
			return "latlon", input, nil
		}
	}
	// date
	if dateRE.MatchString(input) {
		return "date", input, nil
	}
	return "query", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	if uriType != "latlon" {
		return "", errs.Usage("climatemeteo has no resource type %q", uriType)
	}
	parts := strings.SplitN(id, ",", 2)
	if len(parts) != 2 {
		return "", errs.Usage("latlon id must be lat,lon: %q", id)
	}
	lat := strings.TrimSpace(parts[0])
	lon := strings.TrimSpace(parts[1])
	return fmt.Sprintf("https://%s/v1/climate?latitude=%s&longitude=%s", Host, lat, lon), nil
}

// parseLatLon splits "lat,lon" into two float64 values.
func parseLatLon(s string) (lat, lon float64, err error) {
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected lat,lon got %q", s)
	}
	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %w", err)
	}
	lon, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %w", err)
	}
	return lat, lon, nil
}
