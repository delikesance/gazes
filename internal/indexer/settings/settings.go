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
)

type Gateway struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"apiKey"`
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
		if disabled == "true" || (s.optional && disabled != "false" && endpoint == "" && base == "") {
			continue
		}
		gateway, exists := gateways[s.name]
		if endpoint != "" {
			gateway = Gateway{s.name, endpoint, getenv(prefix + "_API_KEY")}
			exists = true
		}
		if exists {
			diagnostics.RegisterSecret(gateway.APIKey)
			client, err := torznab.New(s.name, gateway.Endpoint, gateway.APIKey)
			if err != nil {
				return nil, errors.New("invalid private indexer endpoint")
			}
			providers = append(providers, client)
		} else if path == "" || s.optional {
			if base == "" {
				base = s.base
			}
			providers = append(providers, public.New(s.name, base))
		}
	}
	return providers, nil
}
