package torrent

import (
	"bytes"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const trackerStatusEvery = 15 * time.Second

var (
	statusInfoHash = regexp.MustCompile(`\b[0-9a-f]{40}\b`)
	statusTracker  = regexp.MustCompile(`^\s*"([^"]+)"\s+(next ann:.*last ann: .*)$`)
)

// trackerStatusLoop reports how each tracker answers the client's announces. anacrolix only emits
// status events for websocket trackers, so HTTP and UDP ones (C411, ...) are read from its status
// dump. Only changes are logged, with the tracker URL cut down to its host: private trackers carry
// the passkey in the path.
func (e *ClientEngine) trackerStatusLoop() {
	ticker := time.NewTicker(trackerStatusEvery)
	defer ticker.Stop()
	last := map[string]string{}
	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
		}
		var buf bytes.Buffer
		e.client.WriteStatus(&buf)
		for _, entry := range parseTrackerStatus(buf.String()) {
			// "next ann" is a countdown that changes on every tick: only the outcome of the last announce is news.
			key := entry.infoHash + " " + entry.host
			_, outcome, _ := strings.Cut(entry.status, "last ann: ")
			if last[key] == outcome {
				continue
			}
			last[key] = outcome
			e.logger.Info("torrent.tracker_status", "infohash", entry.infoHash, "tracker", entry.host, "status", entry.status)
		}
	}
}

type trackerStatusEntry struct{ infoHash, host, status string }

// parseTrackerStatus extracts "<tracker> -> next ann / last ann" lines from Client.WriteStatus,
// attributing each to the torrent block it appears under.
func parseTrackerStatus(dump string) []trackerStatusEntry {
	var out []trackerStatusEntry
	infoHash := ""
	for _, line := range strings.Split(dump, "\n") {
		if m := statusTracker.FindStringSubmatch(line); m != nil {
			host := "unknown"
			if raw, err := strconv.Unquote(`"` + m[1] + `"`); err == nil {
				if u, err := url.Parse(raw); err == nil && u.Host != "" {
					host = u.Scheme + "://" + u.Host
				}
			}
			out = append(out, trackerStatusEntry{infoHash, host, m[2]})
			continue
		}
		if h := statusInfoHash.FindString(line); h != "" {
			infoHash = h
		}
	}
	return out
}
