// Package imagecache serves remote artwork (AniList covers and banners) from disk. Each image is
// fetched once, stored forever under a hashed name, and handed to browsers as immutable, so the
// catalog keeps its pictures when the CDN is slow or unreachable and the browser never asks again.
package imagecache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultMaxBytes = 8 << 20
	fetchTimeout    = 15 * time.Second
	// A year is the longest freshness browsers and CDNs honour; the URL embeds the image revision, so
	// a changed picture arrives under a new URL.
	cacheControl = "public, max-age=31536000, immutable"
)

// Options configures a Cache.
type Options struct {
	Dir      string       // where images are stored (created on demand)
	Hosts    []string     // the only hosts that are fetched (exact match, https only)
	MaxBytes int64        // largest image accepted (default 8 MiB)
	Client   *http.Client // upstream client (default: 15 s timeout)
	Logger   *slog.Logger // optional: why an image could not be served
}

// Cache is an http.Handler for GET ?u=<image URL>.
type Cache struct {
	dir      string
	hosts    map[string]bool
	maxBytes int64
	client   *http.Client
	logger   *slog.Logger
	flight   singleflight.Group
}

// New builds a Cache. Redirects are only followed to another allowed host.
func New(o Options) *Cache {
	c := &Cache{dir: o.Dir, hosts: map[string]bool{}, maxBytes: o.MaxBytes, logger: o.Logger}
	for _, h := range o.Hosts {
		c.hosts[strings.ToLower(h)] = true
	}
	if c.maxBytes <= 0 {
		c.maxBytes = defaultMaxBytes
	}
	c.client = o.Client
	if c.client == nil {
		c.client = &http.Client{Timeout: fetchTimeout}
	}
	base := *c.client
	base.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || !c.allowed(req.URL) {
			return errors.New("redirect to a host that is not allowed")
		}
		return nil
	}
	c.client = &base
	return c
}

// Allowed reports whether raw is an image URL this cache is willing to fetch, so callers (and the
// web app) can decide what to route through it.
func (c *Cache) Allowed(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && c.allowed(u)
}

func (c *Cache) allowed(u *url.URL) bool {
	return u.Scheme == "https" && u.User == nil && u.Port() == "" && c.hosts[strings.ToLower(u.Hostname())]
}

var extByType = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif", "image/avif": ".avif"}
var typeByExt = map[string]string{".jpg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".gif": "image/gif", ".avif": "image/avif"}

func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := r.URL.Query().Get("u")
	if !c.Allowed(raw) {
		http.Error(w, "image host not allowed", http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256([]byte(raw))
	name := hex.EncodeToString(sum[:])
	path, ok := c.find(name)
	if !ok {
		res, err, _ := c.flight.Do(name, func() (any, error) { return c.fetch(r.Context(), raw, name) })
		if err != nil {
			var upstream *statusError
			if errors.As(err, &upstream) && upstream.code == http.StatusNotFound {
				http.Error(w, "image not found", http.StatusNotFound)
				return
			}
			if c.logger != nil {
				c.logger.Warn("imagecache.fetch_failed", "url", raw, "err", err)
			}
			http.Error(w, "image unavailable", http.StatusBadGateway)
			return
		}
		path = res.(string)
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "image unavailable", http.StatusBadGateway)
		return
	}
	h := w.Header()
	h.Set("Content-Type", typeByExt[filepath.Ext(path)])
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", `"`+name[:32]+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// find returns the stored file of name, whatever its extension.
func (c *Cache) find(name string) (string, bool) {
	for ext := range typeByExt {
		p := filepath.Join(c.dir, name[:2], name+ext)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

type statusError struct{ code int }

func (e *statusError) Error() string { return fmt.Sprintf("image upstream answered %d", e.code) }

// fetch downloads raw once and stores it. The download is detached from the first caller's request:
// the others waiting on the same image must not lose it because that caller went away.
func (c *Cache) fetch(_ context.Context, raw, name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Gazes/1.0 (AnimeStreamingEngine)")
	req.Header.Set("Accept", "image/*")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &statusError{resp.StatusCode}
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	ext, ok := extByType[mt]
	if !ok {
		return "", fmt.Errorf("image upstream sent %q", mt)
	}
	if err := os.MkdirAll(c.dir, 0o750); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(c.dir, ".fetch-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed
	n, err := io.Copy(tmp, io.LimitReader(resp.Body, c.maxBytes+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	if n > c.maxBytes {
		return "", fmt.Errorf("image larger than %d bytes", c.maxBytes)
	}
	if n == 0 {
		return "", errors.New("empty image")
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		return "", err
	}
	final := filepath.Join(c.dir, name[:2], name+ext)
	if err := os.MkdirAll(filepath.Dir(final), 0o750); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return "", err
	}
	return final, nil
}
