package torrent

import (
	"reflect"
	"testing"
)

func TestSamplePieces(t *testing.T) {
	for _, tc := range []struct {
		begin, end int
		want       []int
	}{{0, 0, nil}, {5, 6, []int{5}}, {5, 7, []int{5, 6}}, {0, 400, []int{0, 200, 399}}} {
		if got := samplePieces(tc.begin, tc.end); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("samplePieces(%d,%d)=%v want %v", tc.begin, tc.end, got, tc.want)
		}
	}
}
