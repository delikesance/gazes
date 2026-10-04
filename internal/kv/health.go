package kv

import (
	"context"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// HealthPolicy tunes the circuit breaker a Governor keeps for one upstream.
//
// closed    -> every call is admitted; failures are counted.
// open      -> after an "immediate" failure (429, 5xx, auth) or Threshold other failures inside
//
//	Window, every call is refused for Base * 2^(step-1), capped at Max (or the upstream's
//	Retry-After when it gave one).
//
// half-open -> once the cooldown ends exactly ONE probe call is admitted. Success closes the
//
//	circuit and resets the backoff; failure re-opens it one step higher.
//
// The state is shared through Redis (one Lua script, so concurrent instances stay consistent) and
// mirrored in-process, which is the only state when Redis is absent or unreachable.
type HealthPolicy struct {
	Threshold    int           // non-immediate failures inside Window that open the circuit
	Window       time.Duration // failure counting window
	Base         time.Duration // first cooldown
	Max          time.Duration // backoff cap (does not limit an upstream's Retry-After)
	ProbeTimeout time.Duration // a half-open probe that never reports frees its slot after this
}

// DefaultHealthPolicy: 3 failures in 30 s, 30 s -> 60 s -> 120 s ... capped at 5 min.
var DefaultHealthPolicy = HealthPolicy{Threshold: 3, Window: 30 * time.Second, Base: 30 * time.Second, Max: 5 * time.Minute, ProbeTimeout: 30 * time.Second}

// maxRetryAfter bounds an upstream-provided Retry-After so a bogus header cannot park a provider.
const maxRetryAfter = time.Hour

// Admission is the answer of Governor.Admit.
type Admission struct {
	Allowed bool
	// Probe is true when this call is the single half-open probe. The caller MUST then report
	// exactly one of Success, Failure or Neutral, otherwise the slot frees itself only after
	// HealthPolicy.ProbeTimeout.
	Probe bool
	// Remaining is how long the circuit stays open (or the probe slot stays taken) when !Allowed.
	Remaining time.Duration
}

// healthState is the in-process breaker. All times are unix milliseconds.
type healthState struct {
	level      int   // consecutive opens since the last success; 0 = closed
	until      int64 // open until
	fails      int
	window     int64 // start of the current failure window
	probeUntil int64 // a probe is in flight until
}

func (p HealthPolicy) backoff(level int) int64 {
	d := float64(p.Base.Milliseconds()) * math.Pow(2, float64(min(level-1, 20)))
	return int64(math.Min(d, float64(p.Max.Milliseconds())))
}

func (s *healthState) peek(now int64) time.Duration {
	if s.level > 0 && now < s.until {
		return time.Duration(s.until-now) * time.Millisecond
	}
	return 0
}

func (s *healthState) admit(now int64, p HealthPolicy) Admission {
	if s.level == 0 {
		return Admission{Allowed: true}
	}
	if now < s.until {
		return Admission{Remaining: time.Duration(s.until-now) * time.Millisecond}
	}
	if now < s.probeUntil {
		return Admission{Remaining: time.Duration(s.probeUntil-now) * time.Millisecond}
	}
	s.probeUntil = now + p.ProbeTimeout.Milliseconds()
	return Admission{Allowed: true, Probe: true}
}

func (s *healthState) fail(now int64, p HealthPolicy, immediate bool, retryAfter time.Duration) time.Duration {
	if s.level > 0 && now < s.until {
		// Already open: a call that started before the circuit opened must not stack backoff steps.
		return time.Duration(s.until-now) * time.Millisecond
	}
	open := immediate || s.level > 0 // level>0 here means the half-open probe failed
	if !open {
		if now-s.window > p.Window.Milliseconds() {
			s.fails, s.window = 0, now
		}
		s.fails++
		open = s.fails >= p.Threshold
	}
	if !open {
		return 0
	}
	s.level++
	s.fails, s.window, s.probeUntil = 0, 0, 0
	d := p.backoff(s.level)
	if retryAfter > 0 {
		d = min(retryAfter, maxRetryAfter).Milliseconds()
	}
	s.until = now + d
	return time.Duration(d) * time.Millisecond
}

// healthScript mirrors healthState on Redis. KEYS[1]=state hash. ARGV: mode (admit|peek|fail|
// success|neutral), threshold, window ms, base ms, max ms, probe ms, immediate 0/1, retryAfter ms.
// Returns {allowed, probe, ms}: ms is the remaining wait (admit/peek) or the cooldown just opened (fail).
var healthScript = redis.NewScript(`
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000 + math.floor(tonumber(t[2]) / 1000)
local mode = ARGV[1]
local threshold, window, base, cap, probeTTL = tonumber(ARGV[2]), tonumber(ARGV[3]), tonumber(ARGV[4]), tonumber(ARGV[5]), tonumber(ARGV[6])
local immediate, retry = tonumber(ARGV[7]), tonumber(ARGV[8])
local s = redis.call('HMGET', KEYS[1], 'level', 'until', 'fails', 'window', 'probe')
local level, untilMs, fails, win, probe = tonumber(s[1]) or 0, tonumber(s[2]) or 0, tonumber(s[3]) or 0, tonumber(s[4]) or 0, tonumber(s[5]) or 0
local function save(keep)
  if level == 0 and fails == 0 then
    redis.call('DEL', KEYS[1])
  else
    redis.call('HSET', KEYS[1], 'level', level, 'until', untilMs, 'fails', fails, 'window', win, 'probe', probe)
    redis.call('PEXPIRE', KEYS[1], math.max(untilMs - now, 0) + keep)
  end
end
local keep = math.max(cap * 2, 600000)
if mode == 'success' then
  level, untilMs, fails, win, probe = 0, 0, 0, 0, 0
  save(keep)
  return {1, 0, 0}
elseif mode == 'neutral' then
  probe = 0
  save(keep)
  return {1, 0, 0}
elseif mode == 'peek' then
  if level > 0 and now < untilMs then return {0, 0, untilMs - now} end
  return {1, 0, 0}
elseif mode == 'admit' then
  if level == 0 then return {1, 0, 0} end
  if now < untilMs then return {0, 0, untilMs - now} end
  if now < probe then return {0, 0, probe - now} end
  probe = now + probeTTL
  save(keep)
  return {1, 1, 0}
end
-- fail
if level > 0 and now < untilMs then return {0, 0, untilMs - now} end
local open = immediate == 1 or level > 0
if not open then
  if now - win > window then fails = 0; win = now end
  fails = fails + 1
  open = fails >= threshold
end
if not open then
  save(keep)
  return {0, 0, 0}
end
level = level + 1
fails, win, probe = 0, 0, 0
local d = math.min(base * 2 ^ math.min(level - 1, 20), cap)
if retry > 0 then d = math.min(retry, 3600000) end
d = math.floor(d)
untilMs = now + d
save(keep)
return {0, 0, d}
`)

// SetHealthPolicy replaces the circuit-breaker tuning (default DefaultHealthPolicy). Call it before use.
func (g *Governor) SetHealthPolicy(p HealthPolicy) { g.health = p }

func (g *Governor) policy() HealthPolicy {
	if g.health.Threshold == 0 {
		return DefaultHealthPolicy
	}
	return g.health
}

func (g *Governor) healthKey() string { return g.key("health") }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// remote runs the shared script; ok=false means Redis is absent or failed and the local state decides.
func (g *Governor) remote(ctx context.Context, mode string, immediate bool, retryAfter time.Duration) (res [3]int64, ok bool) {
	if g.c == nil {
		return res, false
	}
	p := g.policy()
	vals, err := healthScript.Run(ctx, g.c.rdb, []string{g.healthKey()}, mode, p.Threshold, p.Window.Milliseconds(), p.Base.Milliseconds(),
		p.Max.Milliseconds(), p.ProbeTimeout.Milliseconds(), b2i(immediate), retryAfter.Milliseconds()).Int64Slice()
	if err != nil || len(vals) != 3 {
		g.c.stats.errors.Add(1)
		return res, false
	}
	copy(res[:], vals)
	return res, true
}

// Admit asks whether a call may go out. Redis (when reachable) is authoritative; otherwise this
// process decides. Redis errors never refuse a call: the breaker must not become an outage.
func (g *Governor) Admit(ctx context.Context) Admission {
	if r, ok := g.remote(ctx, "admit", false, 0); ok {
		return Admission{Allowed: r[0] == 1, Probe: r[1] == 1, Remaining: time.Duration(r[2]) * time.Millisecond}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.local.admit(time.Now().UnixMilli(), g.policy())
}

// Success closes the circuit and resets the backoff (an empty result list is a success too).
func (g *Governor) Success(ctx context.Context) {
	g.mu.Lock()
	g.local = healthState{}
	g.mu.Unlock()
	g.remote(ctx, "success", false, 0)
}

// Neutral reports a call that says nothing about the upstream's health (a caller that gave up, our
// own timeout on a paced provider). It only releases the half-open probe slot, and only when this
// call actually holds it (probe == Admission.Probe): a stale neutral from a call admitted before
// the circuit opened must not free the slot of a probe that is still in flight.
func (g *Governor) Neutral(ctx context.Context, probe bool) {
	if !probe {
		return
	}
	g.mu.Lock()
	g.local.probeUntil = 0
	g.mu.Unlock()
	g.remote(ctx, "neutral", false, 0)
}

// Failure records an upstream failure. immediate (429, 5xx, auth) opens the circuit at once; any
// other failure counts toward Policy.Threshold inside Policy.Window. A positive retryAfter replaces
// the computed cooldown. It returns the cooldown that was opened, 0 when the circuit stays closed.
func (g *Governor) Failure(ctx context.Context, immediate bool, retryAfter time.Duration) time.Duration {
	g.mu.Lock()
	local := g.local.fail(time.Now().UnixMilli(), g.policy(), immediate, retryAfter)
	g.mu.Unlock()
	if r, ok := g.remote(ctx, "fail", immediate, retryAfter); ok {
		return time.Duration(r[2]) * time.Millisecond
	}
	return local
}

// healthCooldown is the non-mutating remaining open time of the breaker (no probe is taken).
func (g *Governor) healthCooldown(ctx context.Context) time.Duration {
	if r, ok := g.remote(ctx, "peek", false, 0); ok {
		return time.Duration(r[2]) * time.Millisecond
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.local.peek(time.Now().UnixMilli())
}
