package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"

	"github.com/go-chi/chi/v5"
)

const (
	maxPartyRooms   = 500
	maxPartyMembers = 12
)

var (
	partyRoomRE = regexp.MustCompile(`^[a-z0-9]{6,32}$`)
	partyFromRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
)

// partyEvent is a playback action shared with the other viewers of a watch party.
type partyEvent struct {
	From string  `json:"from"`
	Type string  `json:"type"` // play | pause | seek
	T    float64 `json:"t"`    // position in seconds
}

type partyMember struct {
	id string
	ch chan []byte
}

// partyHub relays play/pause/seek between the browsers of a room over server-sent events.
// Rooms live only in memory and vanish with their last viewer.
type partyHub struct {
	mu    sync.Mutex
	rooms map[string]map[*partyMember]struct{}
}

func newPartyHub() *partyHub { return &partyHub{rooms: map[string]map[*partyMember]struct{}{}} }

func (h *partyHub) join(room, id string) *partyMember {
	h.mu.Lock()
	defer h.mu.Unlock()
	members := h.rooms[room]
	if members == nil {
		if len(h.rooms) >= maxPartyRooms {
			return nil
		}
		members = map[*partyMember]struct{}{}
		h.rooms[room] = members
	}
	if len(members) >= maxPartyMembers {
		return nil
	}
	m := &partyMember{id: id, ch: make(chan []byte, 16)}
	members[m] = struct{}{}
	return m
}

func (h *partyHub) leave(room string, m *partyMember) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[room], m)
	if len(h.rooms[room]) == 0 {
		delete(h.rooms, room)
	}
}

func (h *partyHub) publish(room string, ev partyEvent, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for m := range h.rooms[room] {
		if m.id == ev.From {
			continue
		}
		select {
		case m.ch <- payload:
		default: // a stalled viewer must not block the room
		}
	}
}

func (h *partyHub) handleEvents(w http.ResponseWriter, r *http.Request) {
	room, from := chi.URLParam(r, "room"), r.URL.Query().Get("from")
	flusher, ok := w.(http.Flusher)
	if !partyRoomRE.MatchString(room) || !partyFromRE.MatchString(from) || !ok {
		http.Error(w, "invalid party", http.StatusBadRequest)
		return
	}
	m := h.join(room, from)
	if m == nil {
		http.Error(w, "party full", http.StatusServiceUnavailable)
		return
	}
	defer h.leave(room, m)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-m.ch:
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func (h *partyHub) handlePost(w http.ResponseWriter, r *http.Request) {
	room := chi.URLParam(r, "room")
	var ev partyEvent
	if !partyRoomRE.MatchString(room) || json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&ev) != nil ||
		!partyFromRE.MatchString(ev.From) || ev.T < 0 || ev.T > 86400 || (ev.Type != "play" && ev.Type != "pause" && ev.Type != "seek") {
		http.Error(w, "invalid event", http.StatusBadRequest)
		return
	}
	payload, _ := json.Marshal(ev)
	h.publish(room, ev, payload)
	w.WriteHeader(http.StatusNoContent)
}
