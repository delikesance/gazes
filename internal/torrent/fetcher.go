package torrent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/gazes/gazes/internal/diagnostics"
)

// MetainfoFetcher retrieves the .torrent of an infohash from a private provider.
// It returns nil, nil when the provider does not know the torrent.
type MetainfoFetcher func(ctx context.Context, infoHash string) (*metainfo.MetaInfo, error)

const maxMetainfoBytes = 8 << 20

// URLTemplateFetcher downloads template with {infohash} replaced. The URL carries
// credentials, so it is registered as a secret and never put in errors.
func URLTemplateFetcher(client *http.Client, template string) MetainfoFetcher {
	diagnostics.RegisterSecret(template)
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return func(ctx context.Context, infoHash string) (*metainfo.MetaInfo, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.ReplaceAll(template, "{infohash}", infoHash), nil)
		if err != nil {
			return nil, errors.New("invalid metainfo source")
		}
		req.Header.Set("User-Agent", "Mozilla/5.0")
		res, err := client.Do(req)
		if err != nil {
			return nil, errors.New("metainfo source unreachable")
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("metainfo source returned HTTP %d", res.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, maxMetainfoBytes))
		if err != nil {
			return nil, errors.New("metainfo source interrupted")
		}
		mi, err := metainfo.Load(bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("metainfo source did not return a torrent")
		}
		if _, err := mi.UnmarshalInfo(); err != nil || !strings.EqualFold(mi.HashInfoBytes().HexString(), infoHash) {
			return nil, errors.New("metainfo source returned a different torrent")
		}
		return mi, nil
	}
}

// isPrivate reports whether the torrent forbids third-party trackers, DHT and PEX.
func isPrivate(mi *metainfo.MetaInfo) bool {
	info, err := mi.UnmarshalInfo()
	return err == nil && info.Private != nil && *info.Private
}
