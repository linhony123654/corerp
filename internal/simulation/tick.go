package simulation

import (
	"sync"
	"sync/atomic"
	"time"
)

// Loop drives the autonomous world heartbeat.
// Real interval 60s = World advancement 5 minutes.
type Loop struct {
	interval   time.Duration // real-world tick interval
	worldRatio time.Duration // how much world time advances per tick
	tickCount  atomic.Int64
	stopCh     chan struct{}
	handlers   []func()
	mu         sync.Mutex
	stopped    bool
}

func NewLoop(realInterval time.Duration) *Loop {
	if realInterval == 0 {
		realInterval = 60 * time.Second
	}
	return &Loop{
		interval:   realInterval,
		worldRatio: 5 * time.Minute,
		stopCh:     make(chan struct{}),
	}
}

// OnTick registers a handler called on each tick.
func (l *Loop) OnTick(fn func()) {
	l.handlers = append(l.handlers, fn)
}

// Start begins the tick loop in a goroutine.
func (l *Loop) Start() {
	go l.run()
}

// Stop signals the tick loop to terminate. Safe to call multiple times.
func (l *Loop) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopped {
		return
	}
	l.stopped = true
	close(l.stopCh)
}

func (l *Loop) run() {
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.tickCount.Add(1)
			for _, h := range l.handlers {
				h()
			}
		}
	}
}

// TickCount returns how many ticks have fired.
func (l *Loop) TickCount() int {
	return int(l.tickCount.Load())
}

// WorldAdvancement returns total world time advanced so far.
func (l *Loop) WorldAdvancement() time.Duration {
	return time.Duration(l.tickCount.Load()) * l.worldRatio
}
