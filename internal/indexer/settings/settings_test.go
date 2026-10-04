package settings

import (
	"github.com/gazes/gazes/internal/indexer"
	"github.com/gazes/gazes/internal/indexer/torznab"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrivateManifestReplacesDirectProviders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indexers.json")
	os.WriteFile(path, []byte(`[{"name":"anidex","endpoint":"http://prowlarr:9696/7/api","apiKey":"secret"}]`), 0600)
	p, err := Providers(func(k string) string {
		if k == "INDEXER_CONFIG_FILE" {
			return path
		}
		return ""
	})
	if err != nil || len(p) != 1 {
		t.Fatalf("providers=%d err=%v", len(p), err)
	}
	c, ok := p[0].(*torznab.Client)
	if !ok || c.APIKey != "secret" || c.Name() != "anidex" {
		t.Fatal("gateway not loaded")
	}
}
func TestOptionalProvidersDisabledByDefault(t *testing.T) {
	p, err := Providers(func(string) string { return "" })
	if err != nil || len(p) != 2 {
		t.Fatalf("providers=%d err=%v", len(p), err)
	}
	p, err = Providers(func(k string) string {
		if k == "INDEXER_EXT_DISABLED" {
			return "false"
		}
		return ""
	})
	if err != nil || len(p) != 3 {
		t.Fatal("explicit opt-in failed")
	}
}
func TestManifestErrorsDoNotExposeSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indexers.json")
	for _, body := range []string{`sensitive-key`, `[{"name":"anidex","endpoint":"http://sensitive-key@host","apiKey":"sensitive-key"}]`} {
		os.WriteFile(path, []byte(body), 0600)
		_, err := Providers(func(k string) string {
			if k == "INDEXER_CONFIG_FILE" {
				return path
			}
			return ""
		})
		if err == nil || strings.Contains(err.Error(), "sensitive-key") {
			t.Fatal("missing error or leaked secret")
		}
	}
}

func TestConfiguredGatewaysAreNotSilentlyIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indexers.json")
	os.WriteFile(path, []byte(`[{"name":"ext","endpoint":"http://prowlarr:9696/3/api","apiKey":"secret"},{"name":"prowlarr-4","endpoint":"http://prowlarr:9696/4/api","apiKey":"secret"}]`), 0600)
	for _, disabled := range []bool{false, true} {
		p, err := Providers(func(k string) string {
			if k == "INDEXER_CONFIG_FILE" {
				return path
			}
			if disabled && k == "INDEXER_EXT_DISABLED" {
				return "true"
			}
			return ""
		})
		want := 2
		if disabled {
			want = 1
		}
		if err != nil || len(p) != want {
			t.Fatalf("disabled=%v providers=%d err=%v", disabled, len(p), err)
		}
		for _, provider := range p {
			if _, ok := provider.(*torznab.Client); !ok {
				t.Fatal("manifest gateway replaced with direct connector")
			}
		}
	}
}

func gatewayPacing(t *testing.T, manifest string) map[string]indexer.Pacing {
	t.Helper()
	path := filepath.Join(t.TempDir(), "indexers.json")
	os.WriteFile(path, []byte(manifest), 0600)
	p, err := Providers(func(k string) string {
		if k == "INDEXER_CONFIG_FILE" {
			return path
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]indexer.Pacing{}
	for _, provider := range p {
		out[provider.Name()] = provider.(*torznab.Client).Pacing()
	}
	return out
}

func TestC411IsPacedByDefault(t *testing.T) {
	got := gatewayPacing(t, `[{"name":"c411","endpoint":"http://prowlarr:9696/3/api","apiKey":"k","indexerOnly":true},{"name":"anidex","endpoint":"http://prowlarr:9696/7/api","apiKey":"k"}]`)
	if want := (indexer.Pacing{MaxConcurrent: 1, MinInterval: 4100 * time.Millisecond}); got["c411"] != want {
		t.Fatalf("c411 pacing %+v, want %+v", got["c411"], want)
	}
	if !got["anidex"].IsZero() {
		t.Fatalf("anidex must not be paced: %+v", got["anidex"])
	}
}

func TestGatewayPacingConfigOverridesDefault(t *testing.T) {
	got := gatewayPacing(t, `[{"name":"c411","endpoint":"http://prowlarr:9696/3/api","apiKey":"k","maxConcurrent":2,"minIntervalMs":1500},{"name":"torrent9","endpoint":"http://prowlarr:9696/4/api","apiKey":"k","minIntervalMs":2000},{"name":"ext","endpoint":"http://prowlarr:9696/5/api","apiKey":"k","minIntervalMs":-1}]`)
	if want := (indexer.Pacing{MaxConcurrent: 2, MinInterval: 1500 * time.Millisecond}); got["c411"] != want {
		t.Fatalf("c411: %+v", got["c411"])
	}
	if want := (indexer.Pacing{MinInterval: 2 * time.Second}); got["torrent9"] != want {
		t.Fatalf("torrent9: %+v", got["torrent9"])
	}
	if !got["ext"].IsZero() {
		t.Fatalf("negative interval must disable pacing: %+v", got["ext"])
	}
	off := gatewayPacing(t, `[{"name":"c411","endpoint":"http://prowlarr:9696/3/api","apiKey":"k","minIntervalMs":-1}]`)
	if !off["c411"].IsZero() {
		t.Fatalf("c411 default must be disablable: %+v", off["c411"])
	}
}
