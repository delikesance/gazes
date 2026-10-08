package library

import "errors"

// ErrNotRequeueable means the copy is not an original the encoder gave up on.
var ErrNotRequeueable = errors.New("library: copy is not an abandoned encode")

// EncodeQueueState is the part of an entry that decides whether the encoder picks it.
type EncodeQueueState struct {
	EncodeSkipped string `json:"encode_skipped"`
	Attempts      int    `json:"attempts"`
	LastError     string `json:"last_error"`
}

// EncodeQueue returns the queue state of a copy and whether RequeueEncode would accept it: an
// ORIGINAL the encoder abandoned after maxEncodeAttempts failures.
func (s *Service) EncodeQueue(k Key) (EncodeQueueState, bool, error) {
	e, err := s.store.Get(k)
	if err != nil {
		return EncodeQueueState{}, false, err
	}
	return queueState(e), requeueable(e), nil
}

func queueState(e Entry) EncodeQueueState {
	return EncodeQueueState{EncodeSkipped: e.EncodeSkipped, Attempts: e.Attempts, LastError: e.LastError}
}

func requeueable(e Entry) bool { return e.State == StateOriginal && e.EncodeSkipped == "encode_failed" }

// RequeueEncode puts an abandoned original back in the AV1 queue with a fresh attempt count and
// returns its previous queue state (for RestoreEncodeQueue).
func (s *Service) RequeueEncode(k Key) (EncodeQueueState, error) {
	var prev EncodeQueueState
	_, err := s.store.Update(k, func(e *Entry) error {
		if !requeueable(*e) {
			return ErrNotRequeueable
		}
		prev = queueState(*e)
		e.EncodeSkipped, e.Attempts, e.LastError = "", 0, ""
		return nil
	})
	return prev, err
}

// RestoreEncodeQueue takes a copy out of the queue again with its previous state, as long as the
// encoder has not started it since (it is still an ORIGINAL nobody skipped).
func (s *Service) RestoreEncodeQueue(k Key, prev EncodeQueueState) error {
	_, err := s.store.Update(k, func(e *Entry) error {
		if e.State != StateOriginal || e.EncodeSkipped != "" {
			return ErrNotRequeueable
		}
		e.EncodeSkipped, e.Attempts, e.LastError = prev.EncodeSkipped, prev.Attempts, prev.LastError
		return nil
	})
	return err
}
