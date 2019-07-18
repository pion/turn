package turn

import (
	"testing"
	"time"

	"github.com/pion/stun"
	"github.com/pion/turn/internal/proto"
)

func TestAllocationLifeTime(t *testing.T) {
	lifetime := proto.Lifetime{
		Duration: 5 * time.Second,
	}

	m := &stun.Message{}
	lifetimeDuration := allocationLifeTime(m)

	if lifetimeDuration != proto.DefaultLifetime {
		t.Errorf("Allocation lifetime should be default time duration")
	}

	if err := lifetime.AddTo(m); err != nil {
		t.Errorf("Lifetime add to message failed: %v", err)
	}

	lifetimeDuration = allocationLifeTime(m)
	if lifetimeDuration != lifetime.Duration {
		t.Errorf("Expect lifetimeDuration is %s, but %s", lifetime.Duration, lifetimeDuration)
	}

	// If lifetime is bigger than maximumLifetime
	{
		lifetime := proto.Lifetime{
			Duration: maximumLifetime * 2,
		}

		m2 := &stun.Message{}
		_ = lifetime.AddTo(m2)

		lifetimeDuration := allocationLifeTime(m2)
		if lifetimeDuration != proto.DefaultLifetime {
			t.Errorf("Expect lifetimeDuration is %s, but %s", proto.DefaultLifetime, lifetimeDuration)
		}
	}

}
