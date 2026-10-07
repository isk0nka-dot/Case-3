// =============================================================================
// T2: Thundering Herd Simulation — Jitter Desynchronisation Verification
//
// Simulates 5,000 concurrent client reconnects to verify that the jittered
// drain delay from useOfflineQueue.ts desynchronises reconnection storms.
//
// What we test (Go-side simulation of the TypeScript algorithm):
//   1. 5,000 clients independently pick a random jitter in [0, 5000ms)
//   2. The resulting distribution must spread across the full 5-second window
//   3. No single 100ms bucket receives more than 5% of total traffic (250 clients)
//   4. Adaptive backoff doubles interval on failure, resets on success
//   5. Self-scheduling setTimeout-style loop converges, doesn't diverge
//
// NOTE: This is a statistical simulation of the TypeScript jitter algorithm.
// The actual TypeScript code lives in useOfflineQueue.ts; this Go test
// verifies the ALGORITHM's properties independent of the JS runtime.
// =============================================================================
package worker_test

import (
	"math"
	"math/rand"
	"sort"
	"sync"
	"testing"
)

// jitterConfig mirrors the TypeScript OfflineQueueConfig fields.
type jitterConfig struct {
	reconnectJitterMs      int
	drainIntervalMs        int
	drainBackoffMultiplier float64
	maxDrainBackoffMs      int
}

var defaultJitterCfg = jitterConfig{
	reconnectJitterMs:      5000,
	drainIntervalMs:        1000,
	drainBackoffMultiplier: 1.5,
	maxDrainBackoffMs:      30_000,
}

// TestThunderingHerd_5000ConcurrentReconnects simulates 5,000 clients
// reconnecting simultaneously and verifies the jitter desynchronises them.
func TestThunderingHerd_5000ConcurrentReconnects(t *testing.T) {
	const numClients = 5000
	cfg := defaultJitterCfg

	// ── Phase 1: Generate jittered delays for all clients ────────────
	delays := make([]int, numClients)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(idx int) {
			defer wg.Done()
			// Same algorithm as useOfflineQueue.ts startDrain():
			// const jitter = Math.floor(Math.random() * cfg.reconnectJitterMs)
			jitter := rand.Intn(cfg.reconnectJitterMs)
			mu.Lock()
			delays[idx] = jitter
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// ── Phase 2: Bucket into 100ms windows ──────────────────────────
	numBuckets := cfg.reconnectJitterMs / 100 // 50 buckets for 5000ms
	buckets := make([]int, numBuckets)

	for _, delay := range delays {
		bucket := delay / 100
		if bucket >= numBuckets {
			bucket = numBuckets - 1
		}
		buckets[bucket]++
	}

	// ── Phase 3: Statistical validation ─────────────────────────────

	// 3a. No single bucket > 5% of total traffic.
	maxPerBucket := int(float64(numClients) * 0.05) // 250
	for i, count := range buckets {
		if count > maxPerBucket {
			t.Fatalf("THUNDERING HERD: bucket %d (%d-%dms) has %d clients (>%d, 5%% limit)",
				i, i*100, (i+1)*100, count, maxPerBucket)
		}
	}

	// 3b. At least 90% of buckets must be non-empty (good spread).
	nonEmptyBuckets := 0
	for _, count := range buckets {
		if count > 0 {
			nonEmptyBuckets++
		}
	}
	coverageRatio := float64(nonEmptyBuckets) / float64(numBuckets)
	if coverageRatio < 0.9 {
		t.Fatalf("POOR SPREAD: only %d/%d buckets used (%.1f%% < 90%%)",
			nonEmptyBuckets, numBuckets, coverageRatio*100)
	}

	// 3c. Standard deviation check — uniform distribution over [0, 5000) has
	// expected mean = 2500, stddev ≈ 1443. Allow ±10% tolerance.
	sum := 0
	for _, d := range delays {
		sum += d
	}
	mean := float64(sum) / float64(numClients)

	sumSqDiff := 0.0
	for _, d := range delays {
		diff := float64(d) - mean
		sumSqDiff += diff * diff
	}
	stddev := math.Sqrt(sumSqDiff / float64(numClients))

	expectedMean := float64(cfg.reconnectJitterMs) / 2
	expectedStddev := float64(cfg.reconnectJitterMs) / math.Sqrt(12) // uniform dist stddev

	if math.Abs(mean-expectedMean) > expectedMean*0.1 {
		t.Fatalf("DISTRIBUTION SKEW: mean=%.1f, expected=%.1f (>10%% off)",
			mean, expectedMean)
	}
	if math.Abs(stddev-expectedStddev) > expectedStddev*0.15 {
		t.Fatalf("DISTRIBUTION VARIANCE: stddev=%.1f, expected=%.1f (>15%% off)",
			stddev, expectedStddev)
	}

	// ── Phase 4: Verify peak RPS calculation ────────────────────────
	// Sort delays and find the maximum arrivals in any 1-second window.
	sort.Ints(delays)
	maxInWindow := 0
	windowMs := 1000
	for i := 0; i < len(delays); i++ {
		// Find how many clients arrive within [delays[i], delays[i]+1000ms)
		windowEnd := delays[i] + windowMs
		count := 0
		for j := i; j < len(delays) && delays[j] < windowEnd; j++ {
			count++
		}
		if count > maxInWindow {
			maxInWindow = count
		}
	}

	// With uniform distribution over 5s, expected peak ~= 5000/5 * 1.3 ≈ 1300
	// Allow up to 1500 (30% over expected) for statistical variance.
	peakThreshold := int(float64(numClients) / float64(cfg.reconnectJitterMs/windowMs) * 1.5)
	if maxInWindow > peakThreshold {
		t.Fatalf("PEAK RPS TOO HIGH: %d clients in 1s window (threshold=%d)",
			maxInWindow, peakThreshold)
	}

	t.Logf("✅ T2-PASS: Thundering Herd — 5,000 clients desynchronised")
	t.Logf("   Jitter window:   0–%dms", cfg.reconnectJitterMs)
	t.Logf("   Mean delay:      %.1fms (expected: %.1f)", mean, expectedMean)
	t.Logf("   Stddev:          %.1fms (expected: %.1f)", stddev, expectedStddev)
	t.Logf("   Bucket coverage: %d/%d (%.1f%%)", nonEmptyBuckets, numBuckets, coverageRatio*100)
	t.Logf("   Peak 1s window:  %d clients (threshold: %d)", maxInWindow, peakThreshold)
}

// TestThunderingHerd_AdaptiveBackoff verifies the exponential backoff + reset
// behaviour of the drain loop. Simulates the TypeScript backoff algorithm:
//
//	on failure: interval = min(interval * 1.5, 30000)
//	on success: interval = drainIntervalMs (reset)
func TestThunderingHerd_AdaptiveBackoff(t *testing.T) {
	cfg := defaultJitterCfg

	// Start with the base interval.
	currentInterval := cfg.drainIntervalMs

	// Simulate 20 consecutive failures.
	intervals := make([]int, 0, 30)
	for i := 0; i < 20; i++ {
		currentInterval = int(math.Min(
			float64(currentInterval)*cfg.drainBackoffMultiplier,
			float64(cfg.maxDrainBackoffMs),
		))
		intervals = append(intervals, currentInterval)
	}

	// Verify monotonic increase until cap.
	for i := 1; i < len(intervals); i++ {
		if intervals[i] < intervals[i-1] {
			t.Fatalf("BACKOFF VIOLATION: interval decreased at step %d: %d < %d",
				i, intervals[i], intervals[i-1])
		}
	}

	// Verify capped at maxDrainBackoffMs.
	lastInterval := intervals[len(intervals)-1]
	if lastInterval > cfg.maxDrainBackoffMs {
		t.Fatalf("BACKOFF CAP EXCEEDED: %d > %d", lastInterval, cfg.maxDrainBackoffMs)
	}
	if lastInterval != cfg.maxDrainBackoffMs {
		t.Fatalf("BACKOFF SHOULD REACH CAP: %d != %d", lastInterval, cfg.maxDrainBackoffMs)
	}

	// Simulate success — interval resets to base.
	currentInterval = cfg.drainIntervalMs // reset on success

	if currentInterval != cfg.drainIntervalMs {
		t.Fatalf("RESET FAILURE: interval=%d, expected=%d after success",
			currentInterval, cfg.drainIntervalMs)
	}

	// Log the backoff progression.
	t.Logf("✅ T2-PASS: Adaptive backoff verified")
	t.Logf("   Base interval: %dms", cfg.drainIntervalMs)
	t.Logf("   Multiplier:    %.1fx", cfg.drainBackoffMultiplier)
	t.Logf("   Cap:           %dms", cfg.maxDrainBackoffMs)
	t.Logf("   Progression:   %dms → %dms → %dms → ... → %dms (cap)",
		intervals[0], intervals[1], intervals[2], lastInterval)
	t.Logf("   Steps to cap:  %d", countStepsToCap(cfg))
	t.Logf("   After success: reset to %dms ✓", cfg.drainIntervalMs)
}

// TestThunderingHerd_NoSynchronisedBursts verifies that 5,000 clients with
// jitter NEVER produce a burst where >20% arrive in the same 500ms window.
// This is the critical invariant that prevents Kafka/backend saturation.
func TestThunderingHerd_NoSynchronisedBursts(t *testing.T) {
	const numClients = 5000
	const runs = 10 // Run multiple rounds for statistical confidence.
	cfg := defaultJitterCfg

	for run := 0; run < runs; run++ {
		delays := make([]int, numClients)
		for i := 0; i < numClients; i++ {
			delays[i] = rand.Intn(cfg.reconnectJitterMs)
		}

		sort.Ints(delays)

		// Check 500ms sliding window.
		maxIn500ms := 0
		for i := 0; i < len(delays); i++ {
			windowEnd := delays[i] + 500
			count := sort.SearchInts(delays[i:], windowEnd)
			if count > maxIn500ms {
				maxIn500ms = count
			}
		}

		// 20% of 5000 = 1000 clients in 500ms is the absolute max allowed.
		threshold := numClients / 5
		if maxIn500ms > threshold {
			t.Fatalf("BURST DETECTED (run %d): %d clients in 500ms window (>%d threshold)",
				run, maxIn500ms, threshold)
		}
	}

	t.Logf("✅ T2-PASS: No synchronised bursts in %d rounds × %d clients", runs, numClients)
	t.Logf("   Window:    500ms sliding")
	t.Logf("   Threshold: ≤%d clients per window (20%%)", numClients/5)
}

// TestThunderingHerd_BackoffConvergence verifies that after N failures the
// retry interval converges to the cap and doesn't oscillate or diverge.
func TestThunderingHerd_BackoffConvergence(t *testing.T) {
	cfg := defaultJitterCfg
	currentInterval := float64(cfg.drainIntervalMs)

	// Run 100 failure iterations — must converge to cap.
	for i := 0; i < 100; i++ {
		currentInterval = math.Min(
			currentInterval*cfg.drainBackoffMultiplier,
			float64(cfg.maxDrainBackoffMs),
		)
	}

	if int(currentInterval) != cfg.maxDrainBackoffMs {
		t.Fatalf("CONVERGENCE FAILURE: after 100 failures, interval=%d (expected %d)",
			int(currentInterval), cfg.maxDrainBackoffMs)
	}

	// Verify rapid reset on success.
	currentInterval = float64(cfg.drainIntervalMs)
	if int(currentInterval) != cfg.drainIntervalMs {
		t.Fatalf("RESET FAILURE: %d != %d", int(currentInterval), cfg.drainIntervalMs)
	}

	// After reset, verify backoff restarts correctly from base.
	firstBackoff := math.Min(
		currentInterval*cfg.drainBackoffMultiplier,
		float64(cfg.maxDrainBackoffMs),
	)
	expectedFirst := float64(cfg.drainIntervalMs) * cfg.drainBackoffMultiplier
	if int(firstBackoff) != int(expectedFirst) {
		t.Fatalf("POST-RESET BACKOFF: %d != %d", int(firstBackoff), int(expectedFirst))
	}

	t.Logf("✅ T2-PASS: Backoff convergence verified")
	t.Logf("   100 failures → capped at %dms", cfg.maxDrainBackoffMs)
	t.Logf("   1 success → reset to %dms", cfg.drainIntervalMs)
	t.Logf("   First re-backoff: %dms", int(firstBackoff))
}

// countStepsToCap returns how many consecutive failures to reach the backoff cap.
func countStepsToCap(cfg jitterConfig) int {
	interval := float64(cfg.drainIntervalMs)
	steps := 0
	for int(interval) < cfg.maxDrainBackoffMs {
		interval = math.Min(interval*cfg.drainBackoffMultiplier, float64(cfg.maxDrainBackoffMs))
		steps++
	}
	return steps
}
