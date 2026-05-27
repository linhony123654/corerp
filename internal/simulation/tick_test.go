package simulation

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLoopDefaults(t *testing.T) {
	l := NewLoop(0)
	if l.interval != 60*time.Second {
		t.Errorf("default interval = %v, want 60s", l.interval)
	}
	if l.worldRatio != 5*time.Minute {
		t.Errorf("default world ratio = %v, want 5m", l.worldRatio)
	}
}

func TestNewLoopCustomInterval(t *testing.T) {
	l := NewLoop(10 * time.Second)
	if l.interval != 10*time.Second {
		t.Errorf("interval = %v, want 10s", l.interval)
	}
}

func TestTickLoopStartStop(t *testing.T) {
	l := NewLoop(10 * time.Millisecond)
	var count int32

	l.OnTick(func() {
		atomic.AddInt32(&count, 1)
	})

	l.Start()
	time.Sleep(55 * time.Millisecond) // allow ~5 ticks
	l.Stop()

	n := atomic.LoadInt32(&count)
	if n < 3 {
		t.Errorf("expected at least 3 ticks, got %d", n)
	}
}

func TestTickLoopCount(t *testing.T) {
	l := NewLoop(5 * time.Millisecond)
	l.OnTick(func() {})

	l.Start()
	time.Sleep(25 * time.Millisecond)
	l.Stop()

	if l.TickCount() < 3 {
		t.Errorf("tick count = %d, want >= 3", l.TickCount())
	}
}

func TestWorldAdvancement(t *testing.T) {
	l := NewLoop(1 * time.Millisecond)
	l.OnTick(func() {})

	l.Start()
	time.Sleep(10 * time.Millisecond)
	l.Stop()

	adv := l.WorldAdvancement()
	if adv == 0 {
		t.Error("world advancement should not be zero after ticks")
	}
}

func TestMultipleHandlers(t *testing.T) {
	l := NewLoop(5 * time.Millisecond)
	var a, b int32

	l.OnTick(func() { atomic.AddInt32(&a, 1) })
	l.OnTick(func() { atomic.AddInt32(&b, 1) })

	l.Start()
	time.Sleep(25 * time.Millisecond)
	l.Stop()

	ac, bc := atomic.LoadInt32(&a), atomic.LoadInt32(&b)
	if ac == 0 || bc == 0 {
		t.Errorf("both handlers should fire: a=%d b=%d", ac, bc)
	}
	if ac != bc {
		t.Errorf("handlers should fire same number of times: a=%d b=%d", ac, bc)
	}
}

func TestStopBeforeStart(t *testing.T) {
	l := NewLoop(1 * time.Second)
	// Stop before Start should not panic — channel close on nil goroutine
	// Just verify it doesn't hang
	done := make(chan bool)
	go func() {
		l.Stop()
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Stop() before Start() hung")
	}
}

func TestDoubleStop(t *testing.T) {
	l := NewLoop(5 * time.Millisecond)
	l.OnTick(func() {})
	l.Start()
	time.Sleep(10 * time.Millisecond)
	l.Stop()
	// Double stop should not panic
	l.Stop()
}

func TestPauseResume(t *testing.T) {
	l := NewLoop(5 * time.Millisecond)
	var count int32

	l.OnTick(func() {
		atomic.AddInt32(&count, 1)
	})

	l.Start()
	time.Sleep(20 * time.Millisecond) // let some ticks fire

	l.Pause()
	if !l.IsPaused() {
		t.Fatal("expected IsPaused() == true after Pause()")
	}
	snapshot := atomic.LoadInt32(&count)
	time.Sleep(25 * time.Millisecond) // ticks should be skipped
	if got := atomic.LoadInt32(&count); got != snapshot {
		t.Errorf("handlers fired during pause: before=%d after=%d", snapshot, got)
	}

	l.Resume()
	if l.IsPaused() {
		t.Fatal("expected IsPaused() == false after Resume()")
	}
	time.Sleep(20 * time.Millisecond) // ticks should resume
	if got := atomic.LoadInt32(&count); got <= snapshot {
		t.Errorf("handlers did not resume: snapshot=%d current=%d", snapshot, got)
	}

	l.Stop()
}
