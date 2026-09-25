package admin

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// Real goroutines, as the repository's other concurrency tests use: the
// bound is what keeps a screen's reads from taking every database
// connection, and only overlapping work can show it holding.
func TestTogetherRunsEveryReadAtMostAFewAtATime(t *testing.T) {
	var running, peak, done atomic.Int32
	reads := make([]func() error, 12)
	for i := range reads {
		reads[i] = func() error {
			now := running.Add(1)
			for {
				seen := peak.Load()
				if now <= seen || peak.CompareAndSwap(seen, now) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			running.Add(-1)
			done.Add(1)
			return nil
		}
	}
	if err := together(reads...); err != nil {
		t.Fatal(err)
	}
	if done.Load() != 12 {
		t.Fatalf("%d of 12 reads ran", done.Load())
	}
	if peak.Load() > readsAtOnce {
		t.Fatalf("%d reads ran at once, want at most %d", peak.Load(), readsAtOnce)
	}
	if peak.Load() < 2 {
		t.Fatal("the reads ran one after another")
	}
}

// The first error is returned, but only once everything has finished: a read
// still running would be writing into a result the handler has abandoned.
func TestTogetherWaitsForEveryReadBeforeFailing(t *testing.T) {
	broken := errors.New("broken")
	var finished atomic.Bool
	err := together(
		func() error { return broken },
		func() error { time.Sleep(20 * time.Millisecond); finished.Store(true); return nil },
	)
	if !errors.Is(err, broken) {
		t.Fatalf("err = %v, want %v", err, broken)
	}
	if !finished.Load() {
		t.Fatal("returned before the slower read had finished")
	}
}
