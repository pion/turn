// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"sync"
	"time"
)

// PeriodicTimerTimeoutHandler is a handler called on timeout.
type PeriodicTimerTimeoutHandler func(timerID int)

// PeriodicTimer is a periodic timer.
type PeriodicTimer struct {
	id             int
	interval       time.Duration
	timeoutHandler PeriodicTimerTimeoutHandler
	stopFunc       func()
	doneCh         chan struct{}
	mutex          sync.RWMutex
}

// NewPeriodicTimer create a new timer.
func NewPeriodicTimer(id int, timeoutHandler PeriodicTimerTimeoutHandler, interval time.Duration) *PeriodicTimer {
	return &PeriodicTimer{
		id:             id,
		interval:       interval,
		timeoutHandler: timeoutHandler,
	}
}

// Start starts the timer.
func (t *PeriodicTimer) Start() bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// This is a noop if the timer is always running
	if t.stopFunc != nil {
		return false
	}

	cancelCh := make(chan struct{})
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		canceling := false

		for !canceling {
			timer := time.NewTimer(t.interval)

			select {
			case <-timer.C:
				t.timeoutHandler(t.id)
			case <-cancelCh:
				canceling = true
				timer.Stop()
			}
		}
	}()

	t.stopFunc = func() {
		close(cancelCh)
	}
	t.doneCh = doneCh

	return true
}

// Stop stops the timer.
func (t *PeriodicTimer) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.stopFunc != nil {
		t.stopFunc()
		t.stopFunc = nil
	}
}

// StopAndWait stops the timer and waits for its handler goroutine to return.
// It must not be called from the timer's own timeout handler.
func (t *PeriodicTimer) StopAndWait() {
	t.mutex.Lock()
	if t.stopFunc != nil {
		t.stopFunc()
		t.stopFunc = nil
	}
	doneCh := t.doneCh
	t.mutex.Unlock()

	if doneCh != nil {
		<-doneCh
	}
}

// IsRunning tests if the timer is running.
// Debug purpose only.
func (t *PeriodicTimer) IsRunning() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return (t.stopFunc != nil)
}
