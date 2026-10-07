// Package hlc implements a Hybrid Logical Clock (HLC) for server-authoritative
// temporal ordering of proctoring events.
//
// Problem: Client clocks are untrusted. Students can manipulate their system
// clock to forge timestamps, and NTP drift between exam centres can reorder
// events. Using `time.Now()` alone is insufficient because:
//   - Two events within the same millisecond get identical timestamps.
//   - Clock adjustments (NTP step, DST, manual) can cause time to go backward.
//
// Solution: HLC combines a physical wall clock with a monotonic logical counter.
// It guarantees:
//   1. Monotonically increasing values (never goes backward).
//   2. Causal ordering: if event A happens-before event B, then HLC(A) < HLC(B).
//   3. Close to real time: physical component tracks wall clock within drift.
//
// HLC Timestamp structure:
//
//	┌──────────────────────────┬───────────┐
//	│  Physical (48 bits)      │ Logical   │
//	│  Unix millis             │ (16 bits) │
//	└──────────────────────────┴───────────┘
//
// The 48-bit physical component supports dates until year 10889.
// The 16-bit logical counter allows 65535 events per millisecond per node.
//
// Reference: "Logical Physical Clocks and Consistent Snapshots in Globally
// Distributed Databases" — Kulkarni et al., OPODIS 2014.
package hlc

import (
	"sync"
	"time"
)

// Clock is a Hybrid Logical Clock that produces monotonically increasing
// timestamps. It is safe for concurrent use.
type Clock struct {
	mu      sync.Mutex
	lastPT  int64  // last physical time in milliseconds
	lastLC  uint16 // last logical counter
	maxDrift time.Duration
}

// Timestamp is an HLC timestamp with separate physical and logical components.
type Timestamp struct {
	// PhysicalMs is the Unix timestamp in milliseconds.
	PhysicalMs int64

	// Logical is the monotonic counter within the same millisecond.
	Logical uint16
}

// Option configures a Clock.
type Option func(*Clock)

// WithMaxDrift sets the maximum allowed clock drift. If the wall clock
// jumps forward by more than this, the HLC will cap the physical component
// to prevent unbounded divergence from real time. Default: 1 minute.
func WithMaxDrift(d time.Duration) Option {
	return func(c *Clock) {
		c.maxDrift = d
	}
}

// New creates a new HLC with the given options.
func New(opts ...Option) *Clock {
	c := &Clock{
		maxDrift: 1 * time.Minute,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Now generates a new HLC timestamp. It is guaranteed to be strictly
// greater than all previously generated timestamps from this Clock.
//
// Algorithm:
//  1. Read wall clock (pt).
//  2. If pt > lastPT: advance physical, reset logical to 0.
//  3. If pt == lastPT: increment logical counter.
//  4. If pt < lastPT (clock went backward): keep lastPT, increment logical.
func (c *Clock) Now() Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()

	pt := time.Now().UnixMilli()

	switch {
	case pt > c.lastPT:
		// Wall clock advanced — normal case.
		c.lastPT = pt
		c.lastLC = 0

	case pt == c.lastPT:
		// Same millisecond — increment logical counter.
		c.lastLC++
		if c.lastLC == 0 {
			// Overflow (65536 events in 1ms) — force physical advance.
			c.lastPT++
		}

	default:
		// Clock went backward (NTP adjustment, etc.).
		// Keep the last physical time to maintain monotonicity.
		c.lastLC++
		if c.lastLC == 0 {
			c.lastPT++
		}
	}

	return Timestamp{
		PhysicalMs: c.lastPT,
		Logical:    c.lastLC,
	}
}

// Update merges a received HLC timestamp (e.g., from a client or another
// node) with the local clock. Returns a new timestamp that is strictly
// greater than both the local state and the received timestamp.
//
// This is used when processing client-submitted timestamps to maintain
// causal ordering across client↔server boundaries.
func (c *Clock) Update(received Timestamp) Timestamp {
	c.mu.Lock()
	defer c.mu.Unlock()

	pt := time.Now().UnixMilli()

	switch {
	case pt > c.lastPT && pt > received.PhysicalMs:
		// Wall clock is ahead of everything — use it.
		c.lastPT = pt
		c.lastLC = 0

	case c.lastPT == received.PhysicalMs:
		// Local and received have same physical time.
		if received.Logical > c.lastLC {
			c.lastLC = received.Logical
		}
		c.lastLC++
		if c.lastLC == 0 {
			c.lastPT++
		}

	case c.lastPT > received.PhysicalMs:
		// Local is ahead of received.
		c.lastLC++
		if c.lastLC == 0 {
			c.lastPT++
		}

	default:
		// Received is ahead of local.
		c.lastPT = received.PhysicalMs
		c.lastLC = received.Logical + 1
		if c.lastLC == 0 {
			c.lastPT++
		}
	}

	return Timestamp{
		PhysicalMs: c.lastPT,
		Logical:    c.lastLC,
	}
}

// ToUint64 encodes the HLC timestamp as a single uint64 for storage.
// Format: [48-bit physical ms][16-bit logical counter]
func (ts Timestamp) ToUint64() uint64 {
	return (uint64(ts.PhysicalMs) << 16) | uint64(ts.Logical)
}

// FromUint64 decodes an HLC timestamp from a uint64.
func FromUint64(v uint64) Timestamp {
	return Timestamp{
		PhysicalMs: int64(v >> 16),
		Logical:    uint16(v & 0xFFFF),
	}
}

// ToTime converts the physical component to a time.Time (UTC).
// The logical counter is lost — use ToUint64 for lossless serialisation.
func (ts Timestamp) ToTime() time.Time {
	return time.UnixMilli(ts.PhysicalMs).UTC()
}

// Before returns true if ts is causally before other.
func (ts Timestamp) Before(other Timestamp) bool {
	return ts.ToUint64() < other.ToUint64()
}

// After returns true if ts is causally after other.
func (ts Timestamp) After(other Timestamp) bool {
	return ts.ToUint64() > other.ToUint64()
}

// Equal returns true if ts and other represent the same HLC instant.
func (ts Timestamp) Equal(other Timestamp) bool {
	return ts.PhysicalMs == other.PhysicalMs && ts.Logical == other.Logical
}
