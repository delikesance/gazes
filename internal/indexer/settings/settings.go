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
)

type Gateway struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	APIKey   string `json:"apiKey"`
}

var providerName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func gatewayProvider(gateway Gateway) (indexer.Provider, error) {
	diagnostics.RegisterSecret(gateway.APIKey)
	client, err := torznab.New(gateway.Name, gateway.Endpoint, gateway.APIKey)
	if err != nil {
		return nil, errors.New("invalid private indexer endpoint")
	}
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
			gateway = Gateway{s.name, endpoint, getenv(prefix + "_API_KEY")}
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
