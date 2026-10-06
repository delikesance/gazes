// Package vpn rotates the exit IP of the gluetun (Mullvad) sidecar the backend runs behind.
package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrCooldown means a rotation happened too recently to start another one.
var ErrCooldown = errors.New("vpn: rotation cooldown active")

// Rotator restarts the VPN tunnel through gluetun's control server; with several countries in
// SERVER_COUNTRIES gluetun then picks another server, hence another exit IP.
type Rotator struct {
	base      string
	minGap    time.Duration
	http      *http.Client
	now       func() time.Time
	upWait    time.Duration // how long Rotate waits for the new tunnel
	poll      time.Duration
	settle    time.Duration // a rotation this recent counts as "just rotated" for late callers
	failGap   time.Duration // pause after a failed rotation, so a sick gluetun is not hammered
	retryWait time.Duration // pause between attempts to bring the tunnel back up

	mu        sync.Mutex
	last      time.Time // end of the last successful rotation
	notBefore time.Time
}

// New returns a Rotator for the control server at controlURL (e.g. http://127.0.0.1:8000).
// An empty URL gives a disabled Rotator whose Rotate does nothing. minGap is the shortest time
// between two rotations.
func New(controlURL string, minGap time.Duration) *Rotator {
	return &Rotator{
		base:   strings.TrimRight(controlURL, "/"),
		minGap: minGap,
		http:   &http.Client{Timeout: 30 * time.Second},
		now:    time.Now,
		// gluetun's own healthcheck cycles through servers until one answers: that took ~50 s in a test.
		upWait:    90 * time.Second,
		poll:      time.Second,
		settle:    time.Minute,
		failGap:   30 * time.Second,
		retryWait: 2 * time.Second,
	}
}

// Enabled reports whether a control server is configured.
func (r *Rotator) Enabled() bool { return r != nil && r.base != "" }

// Rotate drops the tunnel and brings it back up on a new server, returning once it answers.
//
// It returns nil without doing anything when another caller finished a rotation moments ago (the
// caller was queued behind it and should simply retry on the new IP), and ErrCooldown when the last
// rotation is older than that but still more recent than minGap. A failed rotation arms a short
// pause only.
//
// The rotation never aborts with ctx: a caller giving up (client disconnect) between "stopped" and
// "running" would leave the backend without any network.
func (r *Rotator) Rotate(ctx context.Context) error {
	if !r.Enabled() {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if now.Before(r.notBefore) {
		if !r.last.IsZero() && now.Sub(r.last) < r.settle {
			return nil
		}
		return ErrCooldown
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*r.upWait)
	defer cancel()
	if err := r.restart(ctx); err != nil {
		r.notBefore = r.now().Add(r.failGap)
		return err
	}
	r.last = r.now()
	r.notBefore = r.last.Add(r.minGap)
	return nil
}

func (r *Rotator) restart(ctx context.Context) error {
	if err := r.setStatus(ctx, "stopped"); err != nil {
		return err // still running: nothing to restore
	}
	// From here the backend has no network until "running" succeeds: insist.
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("vpn: restoring the tunnel: %w", ctx.Err())
			case <-time.After(r.retryWait):
			}
		}
		if err = r.setStatus(ctx, "running"); err == nil {
			return r.waitUp(ctx)
		}
	}
	return err
}

// waitUp blocks until the tunnel answers again (gluetun reports a public IP) or ctx / upWait runs out.
func (r *Rotator) waitUp(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, r.upWait)
	defer cancel()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.base+"/v1/publicip/ip", nil)
		if err != nil {
			return err
		}
		if resp, err := r.http.Do(req); err == nil {
			// gluetun answers 200 with an empty public_ip until the new tunnel has fetched it.
			var out struct {
				PublicIP string `json:"public_ip"`
			}
			ok := resp.StatusCode/100 == 2 && json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out) == nil && out.PublicIP != ""
			resp.Body.Close()
			if ok {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("vpn: tunnel did not come back: %w", ctx.Err())
		case <-time.After(r.poll):
		}
	}
}

func (r *Rotator) setStatus(ctx context.Context, status string) error {
	body := fmt.Sprintf(`{"status":%q}`, status)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, r.base+"/v1/vpn/status", bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.http.Do(req)
	if err != nil {
		return fmt.Errorf("vpn: set status %s: %w", status, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("vpn: set status %s: control server answered %d", status, resp.StatusCode)
	}
	return nil
}
