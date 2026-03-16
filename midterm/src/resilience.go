package main

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"
)

type ResilienceConfig struct {
	// toggles
	EnableInjection     bool
	EnableFailFast      bool
	EnableBulkhead      bool
	EnableCircuitBreaker bool

	// injection knobs
	InjectSleepMs int           // e.g., 2000
	InjectFailPct int           // 0-100 e.g., 30

	// fail fast
	TimeoutMs int // e.g., 300

	// bulkhead
	MaxConcurrent int // e.g., 50

	// circuit breaker
	FailThreshold    int // consecutive fails to open, e.g., 20
	OpenDurationMs   int // e.g., 10000
	HalfOpenMaxProbe int // e.g., 5
}

type Resilience struct {
	cfg atomic.Value // stores ResilienceConfig

	sem chan struct{} // bulkhead semaphore

	// circuit breaker: 0 closed, 1 open, 2 half-open
	state            int32
	consecutiveFails int32
	openUntilUnixNs  int64
	halfOpenInFlight int32

	// stats
	externalOK     int64
	externalFail   int64
	fallbackUsed   int64
	cbOpenRejects  int64
	bulkheadReject int64
	timeoutCount   int64
}

var (
	ErrCircuitOpen  = errors.New("circuit open")
	ErrBulkheadFull = errors.New("bulkhead full")
	ErrTimeout      = errors.New("timeout")
)

func NewResilience(cfg ResilienceConfig) *Resilience {
	r := &Resilience{}
	r.cfg.Store(cfg)
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 50
	}
	r.sem = make(chan struct{}, cfg.MaxConcurrent)
	return r
}

func (r *Resilience) UpdateConfig(cfg ResilienceConfig) {
	// if MaxConcurrent changed, rebuild semaphore
	old := r.GetConfig()
	r.cfg.Store(cfg)
	if cfg.MaxConcurrent > 0 && cfg.MaxConcurrent != old.MaxConcurrent {
		r.sem = make(chan struct{}, cfg.MaxConcurrent)
	}
}

func (r *Resilience) GetConfig() ResilienceConfig {
	return r.cfg.Load().(ResilienceConfig)
}

type ResilienceStats struct {
	ExternalOK     int64 `json:"external_ok"`
	ExternalFail   int64 `json:"external_fail"`
	FallbackUsed   int64 `json:"fallback_used"`
	CBOpenRejects  int64 `json:"cb_open_rejects"`
	BulkheadReject int64 `json:"bulkhead_reject"`
	TimeoutCount   int64 `json:"timeout_count"`
	CBState        int32 `json:"cb_state"`
	ConsecFails    int32 `json:"consecutive_fails"`
}

func (r *Resilience) Stats() ResilienceStats {
	return ResilienceStats{
		ExternalOK:     atomic.LoadInt64(&r.externalOK),
		ExternalFail:   atomic.LoadInt64(&r.externalFail),
		FallbackUsed:   atomic.LoadInt64(&r.fallbackUsed),
		CBOpenRejects:  atomic.LoadInt64(&r.cbOpenRejects),
		BulkheadReject: atomic.LoadInt64(&r.bulkheadReject),
		TimeoutCount:   atomic.LoadInt64(&r.timeoutCount),
		CBState:        atomic.LoadInt32(&r.state),
		ConsecFails:    atomic.LoadInt32(&r.consecutiveFails),
	}
}

// simulatedExternal mimics a downstream dependency.
// When injection is enabled:
// - it can fail with InjectFailPct probability
// - it can be slow by sleeping InjectSleepMs
// This allows us to demonstrate failure modes and recovery patterns.
func (r *Resilience) simulatedExternal(ctx context.Context) error {
	cfg := r.GetConfig()
	if !cfg.EnableInjection {
		return nil
	}

	// fail pct
	if cfg.InjectFailPct > 0 {
		if rand.Intn(100) < cfg.InjectFailPct {
			return errors.New("downstream 500")
		}
	}

	// slow sleep
	sleep := time.Duration(cfg.InjectSleepMs) * time.Millisecond
	if sleep <= 0 {
		return nil
	}

	select {
	case <-time.After(sleep):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CallExternal applies resilience patterns around the external call.
// Order:
// 1) Fail Fast: timeout to bound latency
// 2) Circuit Breaker: reject quickly when downstream is unhealthy
// 3) Bulkhead: cap concurrency so slow calls don't exhaust resources
// If any protection triggers, caller should fall back (degraded mode).
func (r *Resilience) CallExternal(parent context.Context) error {
	cfg := r.GetConfig()
	ctx := parent

	// Fail Fast: timeout
	if cfg.EnableFailFast && cfg.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, time.Duration(cfg.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	// Circuit breaker pre-check
	if cfg.EnableCircuitBreaker {
		if err := r.cbPreCheck(); err != nil {
			atomic.AddInt64(&r.cbOpenRejects, 1)
			atomic.AddInt64(&r.fallbackUsed, 1)
			return err
		}
	}

	// Bulkhead limit
	if cfg.EnableBulkhead {
		select {
		case r.sem <- struct{}{}:
			defer func() { <-r.sem }()
		default:
			atomic.AddInt64(&r.bulkheadReject, 1)
			atomic.AddInt64(&r.fallbackUsed, 1)
			if cfg.EnableCircuitBreaker {
				r.cbOnResult(ErrBulkheadFull)
			}
			return ErrBulkheadFull
		}
	}

	// actual call
	err := r.simulatedExternal(ctx)

	// normalize timeout
	if cfg.EnableFailFast && errors.Is(err, context.DeadlineExceeded) {
		atomic.AddInt64(&r.timeoutCount, 1)
		err = ErrTimeout
	}

	// circuit breaker post
	if cfg.EnableCircuitBreaker {
		r.cbOnResult(err)
	}

	if err == nil {
		atomic.AddInt64(&r.externalOK, 1)
	} else {
		atomic.AddInt64(&r.externalFail, 1)
	}

	return err
}

// cbPreCheck enforces circuit breaker state before calling downstream:
// - Open: reject immediately until open duration expires
// - Half-open: allow a limited number of probe requests
// - Closed: allow requests normally
func (r *Resilience) cbPreCheck() error {
	cfg := r.GetConfig()
	now := time.Now().UnixNano()

	state := atomic.LoadInt32(&r.state)
	if state == 1 { // open
		until := atomic.LoadInt64(&r.openUntilUnixNs)
		if now < until {
			return ErrCircuitOpen
		}
		// open expired -> half open
		atomic.StoreInt32(&r.state, 2)
		atomic.StoreInt32(&r.halfOpenInFlight, 0)
	}

	if atomic.LoadInt32(&r.state) == 2 { // half open
		in := atomic.AddInt32(&r.halfOpenInFlight, 1)
		if cfg.HalfOpenMaxProbe > 0 && int(in) > cfg.HalfOpenMaxProbe {
			atomic.AddInt32(&r.halfOpenInFlight, -1)
			return ErrCircuitOpen
		}
	}

	return nil
}

// cbOnResult updates circuit breaker state based on downstream result:
// - success: reset consecutive failures and close circuit
// - failure: increment failure counter and open circuit when threshold reached
func (r *Resilience) cbOnResult(err error) {
	cfg := r.GetConfig()

	// half-open: reduce inflight when done
	if atomic.LoadInt32(&r.state) == 2 {
		defer atomic.AddInt32(&r.halfOpenInFlight, -1)
	}

	if err == nil {
		atomic.StoreInt32(&r.consecutiveFails, 0)
		atomic.StoreInt32(&r.state, 0) // closed
		return
	}

	f := atomic.AddInt32(&r.consecutiveFails, 1)
	if cfg.FailThreshold > 0 && int(f) >= cfg.FailThreshold {
		atomic.StoreInt32(&r.state, 1) // open
		atomic.StoreInt64(&r.openUntilUnixNs,
			time.Now().Add(time.Duration(cfg.OpenDurationMs)*time.Millisecond).UnixNano(),
		)
	}
}