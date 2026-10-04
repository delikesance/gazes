package library

import (
	"errors"
	"fmt"
	"time"
)

// State is the lifecycle stage of one cached episode copy.
type State string

const (
	StateDownloading State = "DOWNLOADING"
	StateOriginal    State = "ORIGINAL"
	StateEncoding    State = "ENCODING"
	StateAV1         State = "AV1"
	StateUnavailable State = "UNAVAILABLE"
)

var (
	ErrExists   = errors.New("library: entry exists")
	ErrNotFound = errors.New("library: entry not found")
)

// Key identifies a copy: one episode of one season in one language ("vostfr" or "vf").
type Key struct {
	SeasonID, Episode int
	Lang              string
}

func (k Key) String() string { return fmt.Sprintf("%d:%d:%s", k.SeasonID, k.Episode, k.Lang) }

// Entry is one row of the library index.
type Entry struct {
	Key
	AnimeID                                                                    int
	Title                                                                      string
	State, PrevState                                                           State
	InfoHash                                                                   string
	FileIndex                                                                  int
	ReleaseName, DiskID, RelPath, SHA256, VideoCodec, LastError, EncodeSkipped string
	SizeBytes, OriginalSizeBytes, ReservedBytes, DurationMS                    int64
	AudioTracks, SubtitleTracks, Attempts                                      int
	CreatedAt, UpdatedAt, LastAccessAt                                         time.Time
}

// Filter narrows List. Zero values mean "any" (Limit 0 = no limit).
type Filter struct {
	States   []State
	DiskID   string
	SeasonID int
	Episode  int
	Limit    int
	OrderBy  string // "updated_at" (default) or "last_access_at"
}

type RecoverReport struct{ Encoding, Downloading int }

// EncoderStatus is the live state of the encoder worker.
type EncoderStatus struct {
	Key       *Key
	Progress  float64
	Paused    bool
	UpdatedAt time.Time
}
