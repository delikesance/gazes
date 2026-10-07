package library

import (
	"errors"
	"testing"
)

func TestRequeueEncodeOnlyTakesAbandonedOriginals(t *testing.T) {
	store, _ := openTest(t)
	svc := &Service{store: store}
	abandoned := Entry{Key: Key{1, 1, "vf"}, State: StateOriginal, EncodeSkipped: "encode_failed", Attempts: 3, LastError: "boom"}
	tests := []struct {
		name string
		e    Entry
	}{
		{"av1", Entry{Key: Key{1, 2, "vf"}, State: StateAV1}},
		{"not smaller", Entry{Key: Key{1, 3, "vf"}, State: StateOriginal, EncodeSkipped: "not_smaller"}},
		{"queued", Entry{Key: Key{1, 4, "vf"}, State: StateOriginal}},
		{"encoding", Entry{Key: Key{1, 5, "vf"}, State: StateEncoding}},
	}
	if err := store.Create(abandoned); err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		if err := store.Create(tc.e); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range tests {
		if _, ok, _ := svc.EncodeQueue(tc.e.Key); ok {
			t.Errorf("%s reported requeueable", tc.name)
		}
		if _, err := svc.RequeueEncode(tc.e.Key); !errors.Is(err, ErrNotRequeueable) {
			t.Errorf("%s: requeue err = %v", tc.name, err)
		}
	}
	if _, err := svc.RequeueEncode(Key{9, 9, "vf"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing copy: %v", err)
	}

	prev, err := svc.RequeueEncode(abandoned.Key)
	if err != nil || prev != (EncodeQueueState{EncodeSkipped: "encode_failed", Attempts: 3, LastError: "boom"}) {
		t.Fatalf("requeue = %+v, %v", prev, err)
	}
	if got, _ := store.Get(abandoned.Key); got.EncodeSkipped != "" || got.Attempts != 0 || got.LastError != "" || got.State != StateOriginal {
		t.Fatalf("after requeue = %+v", got)
	}
	if err := svc.RestoreEncodeQueue(abandoned.Key, prev); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(abandoned.Key); got.EncodeSkipped != "encode_failed" || got.Attempts != 3 {
		t.Fatalf("after restore = %+v", got)
	}
	// once the encoder has taken it, the undo is refused rather than hiding a running encode
	if _, err := svc.RequeueEncode(abandoned.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(abandoned.Key, func(e *Entry) error { e.State = StateEncoding; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := svc.RestoreEncodeQueue(abandoned.Key, prev); !errors.Is(err, ErrNotRequeueable) {
		t.Fatalf("restore while encoding = %v", err)
	}
}
