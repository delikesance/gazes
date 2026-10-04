package torrent

import "testing"

func TestParseTrackerStatusKeepsOnlyTheHost(t *testing.T) {
	dump := "d35413c3eb67d270781259fb2a85b665492ea489: That Time\n" +
		"  trackers:\n" +
		"    \"https://c411.org/announce/SECRETPASSKEY\"\tnext ann: 28m0s, last ann: 12 peers\n" +
		"    \"udp://tracker.example:6969/announce\"\tnext ann: anytime, last ann: bad request\n"
	got := parseTrackerStatus(dump)
	if len(got) != 2 {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	if got[0].infoHash != "d35413c3eb67d270781259fb2a85b665492ea489" || got[0].host != "https://c411.org" || got[0].status != "next ann: 28m0s, last ann: 12 peers" {
		t.Fatalf("bad first entry: %+v", got[0])
	}
	if got[1].host != "udp://tracker.example:6969" {
		t.Fatalf("bad second entry: %+v", got[1])
	}
	for _, e := range got {
		if e.host == "" || e.status == "" || contains(e.host+e.status, "SECRETPASSKEY") {
			t.Fatalf("leaked or empty: %+v", e)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
