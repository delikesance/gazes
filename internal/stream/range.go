package stream

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gazes/gazes/internal/torrent"
)

// ServeRange handles standard HTTP 206 Partial Content range requests for direct-playable video files (.mp4, .webm).
func ServeRange(w http.ResponseWriter, r *http.Request, reader io.ReadSeeker, filename string, size int64) error {
	mimeType := torrent.DetectMimeType(filename)
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-cache")

	// Set Content-Disposition to inline for browser playback
	cleanName := strings.ReplaceAll(filename, `"`, `_`)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, cleanName))

	// Utilize standard library http.ServeContent which flawlessly handles Range, If-Range, Content-Range, and 206 Partial Content
	http.ServeContent(w, r, filename, time.Time{}, reader)
	return nil
}
