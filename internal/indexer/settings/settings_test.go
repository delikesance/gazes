package settings

import (
	"github.com/gazes/gazes/internal/indexer/torznab"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
