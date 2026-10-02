package torrent

import (
	"path/filepath"
	"strings"
)

// VideoExtensions lists supported video container formats.
var VideoExtensions = map[string]string{
	".mkv":  "video/x-matroska",
	".mp4":  "video/mp4",
	".m4v":  "video/mp4",
	".webm": "video/webm",
	".avi":  "video/x-msvideo",
	".ts":   "video/mp2t",
	".mov":  "video/quicktime",
	".flv":  "video/x-flv",
	".wmv":  "video/x-ms-wmv",
}

// IsVideoFile returns true if the file path has a known video extension.
func IsVideoFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	_, ok := VideoExtensions[ext]
	return ok
}

// DetectMimeType returns the MIME type for a given filename or fallback default.
func DetectMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if mime, ok := VideoExtensions[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

// FindMainVideoFile identifies the best candidate video file in a torrent (largest video file).
func FindMainVideoFile(files []FileInfo) *FileInfo {
	var bestCandidate *FileInfo
	var maxLen int64 = -1

	for i := range files {
		f := &files[i]
		if f.IsVideo && f.Length > maxLen {
			maxLen = f.Length
			bestCandidate = f
		}
	}

	return bestCandidate
}
