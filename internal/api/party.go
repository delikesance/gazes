package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	maxPartyRooms        = 500
	maxPartyMembers      = 12
	maxPartyStreamsPerIP = 6 // a household joins from one address; more is one client hoarding rooms
)

// partyHeartbeat keeps quiet rooms alive through Next's rewrite proxy, which drops streams idle for 30 s.
var partyHeartbeat = 15 * time.Second

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

// partyState is a room's last known playback, replayed to a viewer who joins mid-episode.
type partyState struct {
	typ string // play | pause | seek ("seek" until someone plays or pauses)
	t   float64
	at  time.Time
}

type partyMember struct {
	id, ip string
	ch     chan []byte
}

// partyHub relays play/pause/seek between the browsers of a room over server-sent events.
// Rooms live only in memory and vanish with their last viewer. Anyone holding a room code (about
// 41 random bits, see the web client) can post to it under any "from": like the link, it is the key.
type partyHub struct {
	mu       sync.Mutex
	rooms    map[string]map[*partyMember]struct{}
	state    map[string]partyState
	streams  map[string]int               // open event streams per client (IP, or /64 for IPv6)
	clientIP func(r *http.Request) string // set by the server; nil means the socket peer
}

func newPartyHub() *partyHub {
	return &partyHub{rooms: map[string]map[*partyMember]struct{}{}, state: map[string]partyState{}, streams: map[string]int{}}
}

func (h *partyHub) join(room, id, ip string) *partyMember {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.streams[ip] >= maxPartyStreamsPerIP {
		return nil
	}
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
	m := &partyMember{id: id, ip: ip, ch: make(chan []byte, 16)}
	members[m] = struct{}{}
	h.streams[ip]++
	return m
}

func (h *partyHub) leave(room string, m *partyMember) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[room], m)
	if len(h.rooms[room]) == 0 {
		delete(h.rooms, room)
		delete(h.state, room)
	}
	if h.streams[m.ip]--; h.streams[m.ip] <= 0 {
		delete(h.streams, m.ip)
	}
}

func (h *partyHub) publish(room string, ev partyEvent, payload []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[room] == nil {
		return
	}
	st := partyState{typ: ev.Type, t: ev.T, at: time.Now()}
	if prev, ok := h.state[room]; ok && ev.Type == "seek" && prev.typ != "seek" {
		st.typ = prev.typ // a seek keeps the room playing or paused
	}
	h.state[room] = st
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

// snapshot is the room's current playback for a new viewer: position advanced by the time spent playing.
func (h *partyHub) snapshot(room string) (partyEvent, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.state[room]
	if !ok {
		return partyEvent{}, false
	}
	ev := partyEvent{Type: st.typ, T: st.t}
	if st.typ == "play" {
		ev.T += time.Since(st.at).Seconds()
	}
	return ev, true
}

func (h *partyHub) handleEvents(w http.ResponseWriter, r *http.Request) {
	room, from := chi.URLParam(r, "room"), r.URL.Query().Get("from")
	flusher, ok := w.(http.Flusher)
	if !partyRoomRE.MatchString(room) || !partyFromRE.MatchString(from) || !ok {
		http.Error(w, "invalid party", http.StatusBadRequest)
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	ip = clientBucket(ip)
	if h.clientIP != nil {
		ip = h.clientIP(r)
	}
	m := h.join(room, from, ip)
	if m == nil {
		http.Error(w, "party full", http.StatusServiceUnavailable)
		return
	}
	defer h.leave(room, m)
	// The server's WriteTimeout is sized for video; a party stream lives as long as the room.
	// The heartbeat and the client disconnect bound it instead.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	if ev, ok := h.snapshot(room); ok {
		payload, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", payload)
	}
	flusher.Flush()
	heartbeat := time.NewTicker(partyHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
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
