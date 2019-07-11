package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTryLock(t *testing.T) {
	t.Run("success case", func(t *testing.T) {
		cl := &TryLock{}
		testFunc := func() error {
			if err := cl.Lock(); err != nil {
				return err
			}
			defer cl.Unlock()
			return nil
		}

		err := testFunc()
		assert.NoError(t, err, "should succeed")
		assert.Equal(t, int32(0), cl.n, "should match")
	})

	t.Run("failure case", func(t *testing.T) {
		cl := &TryLock{}
		testFunc := func() error {
			if err := cl.Lock(); err != nil {
				return err
			}
			defer cl.Unlock()
			time.Sleep(50 * time.Millisecond)
			return nil
		}

		var err1, err2 error
		doneCh1 := make(chan struct{})
		doneCh2 := make(chan struct{})

		go func() {
			err1 = testFunc()
			close(doneCh1)
		}()
		go func() {
			err2 = testFunc()
			close(doneCh2)
		}()

		<-doneCh1
		<-doneCh2

		// Either one of them should fail
		if err1 == nil {
			assert.Error(t, err2, "should fail")
		} else {
			assert.Error(t, err1, "should fail")
		}
		assert.Equal(t, int32(0), cl.n, "should match")
	})
}
