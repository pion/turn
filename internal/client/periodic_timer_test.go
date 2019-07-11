package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPriodicTimer(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		timerID := 3
		var nCbs int
		rt := NewPeriodicTimer(timerID, func(id int) {
			nCbs++
			assert.Equal(t, timerID, id)
		}, 50*time.Millisecond)

		assert.False(t, rt.IsRunning(), "should not be running yet")

		ok := rt.Start()
		assert.True(t, ok, "should be true")
		assert.True(t, rt.IsRunning(), "should be running")

		time.Sleep(100 * time.Millisecond)

		ok = rt.Start()
		assert.False(t, ok, "start again is noop")

		time.Sleep(120 * time.Millisecond)
		rt.Stop()
		assert.False(t, rt.IsRunning(), "should not be running")
		assert.Equal(t, 4, nCbs, "should be called 4 times (actual: %d)", nCbs)
	})

	t.Run("stop inside handler", func(t *testing.T) {
		timerID := 4
		var rt *PeriodicTimer
		rt = NewPeriodicTimer(timerID, func(id int) {
			assert.Equal(t, timerID, id)
			rt.Stop()
		}, 20*time.Millisecond)

		assert.False(t, rt.IsRunning(), "should not be running yet")

		ok := rt.Start()
		assert.True(t, ok, "should be true")
		assert.True(t, rt.IsRunning(), "should be running")
		time.Sleep(30 * time.Millisecond)
		assert.False(t, rt.IsRunning(), "should not be running")
	})
}
