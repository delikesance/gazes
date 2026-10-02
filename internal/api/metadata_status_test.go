package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gazes/gazes/internal/metadata"
	"github.com/gazes/gazes/internal/torrent"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type probeTestReader struct{ *strings.Reader }

func (probeTestReader) Close() error { return nil }

type probeTestEngine struct{ packEngine }

func (*probeTestEngine) GetFileStream(context.Context, string, int) (io.ReadSeekCloser, *torrent.FileInfo, error) {
	return probeTestReader{strings.NewReader("header")}, &torrent.FileInfo{Length: 6}, nil
}

type statusAnalyzer struct {
	result *metadata.VideoMetadata
	err    error
}

func (a statusAnalyzer) ProbeReader(context.Context, io.Reader, int64) (*metadata.VideoMetadata, error) {
	return a.result, a.err
}

func TestMetadataProbeStatusSeparatesTimeoutFailureAndSuccess(t *testing.T) {
	for _, tc := range []struct {
		status string
		meta   *metadata.VideoMetadata
		err    error
	}{
		{"timeout", nil, context.DeadlineExceeded},
		{"failed", nil, errors.New("invalid container")},
		{"complete", &metadata.VideoMetadata{VideoCodec: "h264", AudioTracks: []metadata.AudioTrack{{Index: 0, Language: "jpn"}, {Index: 1, Language: "fre"}}}, nil},
	} {
		t.Run(tc.status, func(t *testing.T) {
			server := &Server{torrentEngine: &probeTestEngine{}, analyzer: statusAnalyzer{tc.meta, tc.err}}
			rr := httptest.NewRecorder()
			server.HandleMetadata(rr, httptest.NewRequest("GET", "/metadata?ih=test&file_idx=24", nil))
			var result metadata.VideoMetadata
			if rr.Code != 200 || json.Unmarshal(rr.Body.Bytes(), &result) != nil || result.ProbeStatus != tc.status {
				t.Fatalf("%d %s", rr.Code, rr.Body.String())
			}
			if tc.status != "complete" && (result.ProbeErrorCode == "" || len(result.AudioTracks) > 0) {
				t.Fatalf("failed probe claimed audio evidence: %+v", result)
			}
		})
	}
}
