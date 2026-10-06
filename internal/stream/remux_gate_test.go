package stream

import "testing"

func TestRemuxGateCapsPerClientAndGlobally(t *testing.T) {
	var releases []func()
	for range maxRemuxesPerClient {
		release, ok := acquireRemux("203.0.113.7")
		if !ok {
			t.Fatal("refused under the per-client cap")
		}
		releases = append(releases, release)
	}
	// One caller must not hold every slot.
	if _, ok := acquireRemux("203.0.113.7"); ok {
		t.Fatal("client exceeded its remux cap")
	}
	releases[0]()
	releases = releases[1:]
	if release, ok := acquireRemux("203.0.113.7"); !ok {
		t.Fatal("a finished remux did not free its slot")
	} else {
		releases = append(releases, release)
	}
	for i := len(releases); i < maxRemuxes; i++ {
		release, ok := acquireRemux("")
		if !ok {
			t.Fatalf("global slot %d refused", i)
		}
		releases = append(releases, release)
	}
	if _, ok := acquireRemux("198.51.100.1"); ok {
		t.Fatal("global cap exceeded")
	}
	for _, release := range releases {
		release()
	}
	if remuxGate.total != 0 || len(remuxGate.perClient) != 0 {
		t.Fatalf("slots leaked: %d, %v", remuxGate.total, remuxGate.perClient)
	}
}
