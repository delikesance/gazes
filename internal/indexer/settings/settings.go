// Package settings loads private gateway configuration without exposing credentials in errors.
package settings

import (
	"encoding/json"
	"errors"
	"github.com/gazes/gazes/internal/diagnostics"
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/indexer/public"
	"github.com/gazes/gazes/internal/indexer/torznab"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Gateway struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"apiKey"`
	// IndexerOnly gateways front private trackers: their results are a VF fallback
	// and their magnets ask the backend to fetch the .torrent (see torznab.SourcePrefix).
	IndexerOnly bool `json:"indexerOnly,omitempty"`
	// MaxConcurrent and MinIntervalMs pace calls to the gateway (see indexer.Pacing). Both zero
	// means "use the built-in default for this gateway name" (defaultPacing); a negative
	// MinIntervalMs switches pacing off, even for a gateway that has a default.
	MaxConcurrent int `json:"maxConcurrent,omitempty"`
	MinIntervalMs int `json:"minIntervalMs,omitempty"`
}

// defaultPacing is the pacing of gateways that need one and have no explicit configuration.
// C411 is reached through Prowlarr, whose C411 definition enforces requestDelay 4.1 s: concurrent
// calls queue inside Prowlarr and time out on our side, so it gets one request at a time, 4.1 s
// apart. The gateway is identified by its (lower-cased) name "c411", the name the Prowlarr
// bootstrap (deploy/prowlarr-bootstrap.py) gives it in indexers.json.
func defaultPacing(name string) indexer.Pacing {
	if name == "c411" {
		return indexer.Pacing{MaxConcurrent: 1, MinInterval: 4100 * time.Millisecond}
	}
	return indexer.Pacing{}
}

// pacing resolves a gateway's pacing: explicit configuration wins over the name-based default.
func (g Gateway) pacing() indexer.Pacing {
	if g.MaxConcurrent == 0 && g.MinIntervalMs == 0 {
		return defaultPacing(g.Name)
	}
	if g.MinIntervalMs < 0 {
		return indexer.Pacing{}
	}
	return indexer.Pacing{MaxConcurrent: min(max(g.MaxConcurrent, 0), 16), MinInterval: time.Duration(min(g.MinIntervalMs, 60_000)) * time.Millisecond}
}

var providerName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func gatewayProvider(gateway Gateway) (indexer.Provider, error) {
	diagnostics.RegisterSecret(gateway.APIKey)
	client, err := torznab.New(gateway.Name, gateway.Endpoint, gateway.APIKey)
	if err != nil {
		return nil, errors.New("invalid private indexer endpoint")
	}
	client.IndexerOnly = gateway.IndexerOnly
	client.Pace = gateway.pacing()
	return client, nil
}

func Providers(getenv func(string) string) ([]indexer.Provider, error) {
	gateways := map[string]Gateway{}
	path := getenv("INDEXER_CONFIG_FILE")
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.New("cannot read private indexer configuration")
		}
		var entries []Gateway
		if json.Unmarshal(data, &entries) != nil {
			return nil, errors.New("invalid private indexer configuration")
		}
		for _, entry := range entries {
			entry.Name = strings.ToLower(strings.TrimSpace(entry.Name))
			if _, duplicate := gateways[entry.Name]; duplicate || !providerName.MatchString(entry.Name) {
				return nil, errors.New("invalid private indexer configuration")
			}
			gateways[entry.Name] = entry
		}
	}
	var providers []indexer.Provider
	for _, s := range []struct {
		name, env, base string
		optional        bool
	}{
		{"ext", "EXT", "https://ext.to", true}, {"magnetdl", "MAGNETDL", "https://www.magnetdl.com", true},
		{"thepiratebay", "THEPIRATEBAY", "https://apibay.org", false}, {"anidex", "ANIDEX", "https://anidex.info", false},
	} {
		prefix := "INDEXER_" + s.env
		disabled := getenv(prefix + "_DISABLED")
		endpoint := getenv(prefix + "_TORZNAB_URL")
		base := getenv(prefix + "_URL")
		gateway, exists := gateways[s.name]
		delete(gateways, s.name)
		if disabled == "true" || (s.optional && !exists && disabled != "false" && endpoint == "" && base == "") {
			continue
		}
		if endpoint != "" {
			gateway = Gateway{Name: s.name, Endpoint: endpoint, APIKey: getenv(prefix + "_API_KEY")}
			exists = true
		}
		if exists {
			client, err := gatewayProvider(gateway)
			if err != nil {
				return nil, err
			}
			providers = append(providers, client)
		} else if path == "" || s.optional {
			if base == "" {
				base = s.base
			}
			providers = append(providers, public.New(s.name, base))
		}
	}
	// Every explicitly configured gateway participates, including indexers added
	// manually in Prowlarr. Stable names keep provider caches and ordering stable.
	names := make([]string, 0, len(gateways))
	for name := range gateways {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if getenv("INDEXER_"+strings.ToUpper(strings.ReplaceAll(name, "-", "_"))+"_DISABLED") == "true" {
			continue
		}
		client, err := gatewayProvider(gateways[name])
		if err != nil {
			return nil, err
		}
		providers = append(providers, client)
	}
	return providers, nil
}
