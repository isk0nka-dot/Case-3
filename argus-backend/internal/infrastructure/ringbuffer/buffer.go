// Package ringbuffer provides a fixed-size circular buffer for storing
// video segments. It maintains a rolling window of recent video data
// so that when a violation event occurs, the pre-capture window (30s before)
// is immediately available without needing to re-request video data.
//
// Memory usage: ~50KB/segment at 480p × 60 segments = ~3MB per session.
// This is significantly more memory-efficient than storing full video streams.
//
// Thread safety: RWMutex — write lock for Push, read lock for Snapshot.
package ringbuffer

import (
	"sync"
	"time"
)

// Segment represents a single video chunk in the ring buffer.
// Each segment contains approximately 1 second of video data.
type Segment struct {
	// Data is the raw video bytes (WebM/MP4 segment).
	Data []byte

	// Timestamp is when this segment was captured.
	Timestamp time.Time

	// Duration is the playback duration of this segment.
	Duration time.Duration
}

// Buffer is a fixed-size circular buffer of video segments.
// When the buffer is full, the oldest segment is overwritten.
//
// Default capacity: 60 segments × ~1s = 60-second rolling window.
type Buffer struct {
	mu       sync.RWMutex
	segments []Segment
	head     int  // Next write position.
	count    int  // Number of valid segments.
	capacity int  // Maximum segments.
}

// New creates a new ring buffer with the given capacity.
// Typical capacity: 60 (for a 60-second rolling window).
func New(capacity int) *Buffer {
	if capacity < 1 {
		capacity = 60
	}
	return &Buffer{
		segments: make([]Segment, capacity),
		capacity: capacity,
	}
}

// Push adds a new segment to the ring buffer. If the buffer is full,
// the oldest segment is overwritten. This operation is O(1).
//
// The data slice is copied to prevent external mutation.
func (b *Buffer) Push(data []byte, timestamp time.Time, duration time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Copy the data to prevent external mutation.
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	b.segments[b.head] = Segment{
		Data:      dataCopy,
		Timestamp: timestamp,
		Duration:  duration,
	}

	b.head = (b.head + 1) % b.capacity
	if b.count < b.capacity {
		b.count++
	}
}

// Snapshot returns a copy of all segments within the time window defined by
// [eventTime - beforeSec, eventTime + afterSec]. The returned segments are
// sorted by timestamp (oldest first).
//
// This is a read-only operation that does not modify the buffer.
func (b *Buffer) Snapshot(eventTime time.Time, beforeSec float64, afterSec float64) []Segment {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 {
		return nil
	}

	windowStart := eventTime.Add(-time.Duration(beforeSec * float64(time.Second)))
	windowEnd := eventTime.Add(time.Duration(afterSec * float64(time.Second)))

	// Collect segments within the window.
	result := make([]Segment, 0, b.count)

	// Walk the ring buffer from oldest to newest.
	start := 0
	if b.count == b.capacity {
		start = b.head // When full, head points to the oldest entry.
	}

	for i := 0; i < b.count; i++ {
		idx := (start + i) % b.capacity
		seg := b.segments[idx]

		if seg.Timestamp.Before(windowStart) || seg.Timestamp.After(windowEnd) {
			continue
		}

		// Copy segment data to prevent race conditions with Push().
		dataCopy := make([]byte, len(seg.Data))
		copy(dataCopy, seg.Data)

		result = append(result, Segment{
			Data:      dataCopy,
			Timestamp: seg.Timestamp,
			Duration:  seg.Duration,
		})
	}

	return result
}

// Len returns the number of valid segments in the buffer.
func (b *Buffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// TotalBytes returns the approximate total memory used by segment data.
func (b *Buffer) TotalBytes() int64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var total int64
	start := 0
	if b.count == b.capacity {
		start = b.head
	}
	for i := 0; i < b.count; i++ {
		idx := (start + i) % b.capacity
		total += int64(len(b.segments[idx].Data))
	}
	return total
}

// Reset clears all segments from the buffer.
func (b *Buffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.segments {
		b.segments[i] = Segment{}
	}
	b.head = 0
	b.count = 0
}
