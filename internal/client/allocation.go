// SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/pion/logging"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v5"
	"github.com/pion/turn/v5/internal/proto"
)

// AllocationConfig is a set of configuration params use by NewUDPConn and NewTCPAllocation.
type AllocationConfig struct {
	Client                    Client
	RelayedAddr               net.Addr
	ServerAddr                net.Addr
	Integrity                 stun.MessageIntegrity
	Nonce                     stun.Nonce
	Username                  stun.Username
	Realm                     stun.Realm
	Lifetime                  time.Duration
	Net                       transport.Net
	Log                       logging.LeveledLogger
	PermissionRefreshInterval time.Duration
	BindingRefreshInterval    time.Duration
	BindingCheckInterval      time.Duration
}

type allocation struct {
	client            Client                // Read-only
	relayedAddr       net.Addr              // Read-only
	serverAddr        net.Addr              // Read-only
	permMap           *permissionMap        // Thread-safe
	integrity         stun.MessageIntegrity // Read-only
	username          stun.Username         // Read-only
	realm             stun.Realm            // Read-only
	_nonce            stun.Nonce            // Needs mutex x
	_lifetime         time.Duration         // Needs mutex x
	net               transport.Net         // Thread-safe
	refreshAllocTimer *PeriodicTimer        // Thread-safe
	refreshPermsTimer *PeriodicTimer        // Thread-safe
	readTimer         *time.Timer           // Thread-safe
	mutex             sync.RWMutex          // Thread-safe
	log               logging.LeveledLogger // Read-only
}

func (a *allocation) setNonceFromMsg(msg *stun.Message) {
	// Update nonce
	var nonce stun.Nonce
	if err := nonce.GetFrom(msg); err == nil {
		a.setNonce(nonce)
		a.log.Debug("Refresh allocation: 438, got new nonce.")
	} else {
		a.log.Warn("Refresh allocation: 438 but no nonce.")
	}
}

func (a *allocation) refreshAllocation(lifetime time.Duration, dontWait bool) error {
	msg, err := stun.Build(
		stun.TransactionID,
		stun.NewType(stun.MethodRefresh, stun.ClassRequest),
		proto.Lifetime{Duration: lifetime},
		a.username,
		a.realm,
		a.nonce(),
		a.integrity,
		stun.Fingerprint,
	)
	if err != nil {
		return fmt.Errorf("%w: %s", errFailedToBuildRefreshRequest, err.Error())
	}

	a.log.Debugf("Send refresh request (dontWait=%v)", dontWait)
	trRes, err := a.client.PerformTransactionWithContext(msg, a.serverAddr, dontWait, context.TODO())
	if err != nil {
		return fmt.Errorf("%w: %s", errFailedToRefreshAllocation, err.Error())
	}

	if dontWait {
		a.log.Debug("Refresh request sent")

		return nil
	}

	a.log.Debug("Refresh request sent, and waiting response")

	res := trRes.Msg
	if res.Type.Class == stun.ClassErrorResponse {
		var code stun.ErrorCodeAttribute
		if err = code.GetFrom(res); err == nil {
			if code.Code == stun.CodeStaleNonce {
				a.setNonceFromMsg(res)

				return errTryAgain
			}

			// err is nil here: returning it reported a rejected refresh as a success.
			return &stun.TurnError{StunMessageType: res.Type, ErrorCodeAttr: code}
		}

		return fmt.Errorf("%s", res.Type) //nolint:err113
	}

	// Getting lifetime from response
	var updatedLifetime proto.Lifetime
	if err := updatedLifetime.GetFrom(res); err != nil {
		return fmt.Errorf("%w: %s", errFailedToGetLifetime, err.Error())
	}

	a.setLifetime(updatedLifetime.Duration)
	a.log.Debugf("Updated lifetime: %d seconds", int(a.lifetime().Seconds()))

	return nil
}

func (a *allocation) refreshPermissions() error {
	addrs := a.permMap.addrs()
	if len(addrs) == 0 {
		a.log.Debug("No permission to refresh")

		return nil
	}
	if err := a.CreatePermissions(addrs...); err != nil {
		if errors.Is(err, errTryAgain) {
			return errTryAgain
		}
		a.log.Errorf("Fail to refresh permissions: %s", err)

		return err
	}
	a.log.Debug("Refresh permissions successful")

	return nil
}

func (a *allocation) onRefreshTimers(id int) {
	a.log.Debugf("Refresh timer %d expired", id)
	switch id {
	case timerIDRefreshAlloc:
		var err error
		lifetime := a.lifetime()
		// Limit the max retries on errTryAgain to 3
		// when stale nonce returns, sencond retry should succeed
		for range maxRetryAttempts {
			err = a.refreshAllocation(lifetime, false)
			if !errors.Is(err, errTryAgain) {
				break
			}
		}
		if err != nil {
			a.log.Warnf("Failed to refresh allocation: %s", err)
			a.stopRefreshingIfFinal(err)
		}
	case timerIDRefreshPerms:
		var err error
		for range maxRetryAttempts {
			err = a.refreshPermissions()
			if !errors.Is(err, errTryAgain) {
				break
			}
		}
		if err != nil {
			a.log.Warnf("Failed to refresh permissions: %s", err)
			a.stopRefreshingIfFinal(err)
		}
	}
}

// stopRefreshingIfFinal stops both refresh timers when err says no later refresh
// of this allocation can succeed. Without it a client whose credentials expired
// (TURN REST API usernames carry an expiry), or whose allocation the server lost
// (a server restart), refreshes for the life of the allocation and every attempt
// is rejected.
func (a *allocation) stopRefreshingIfFinal(err error) {
	if !isFinalRefreshError(err) {
		return
	}
	a.log.Warnf("Stopped refreshing the allocation: %s", err)
	a.refreshAllocTimer.Stop()
	a.refreshPermsTimer.Stop()
}

// isFinalRefreshError reports whether a refresh failed in a way that retrying
// with the same allocation and credentials cannot fix. A stale nonce is not one
// (it is retried with the new nonce), and neither is a timeout (the network may
// come back).
func isFinalRefreshError(err error) bool {
	var turnErr *stun.TurnError
	if errors.As(err, &turnErr) {
		switch turnErr.ErrorCodeAttr.Code {
		case stun.CodeBadRequest, stun.CodeUnauthorized, stun.CodeForbidden, stun.CodeAllocMismatch:
			return true
		}

		return false
	}

	return errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}

func (a *allocation) nonce() stun.Nonce {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a._nonce
}

func (a *allocation) setNonce(nonce stun.Nonce) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.log.Debugf("Set new nonce with %d bytes", len(nonce))
	a._nonce = nonce
}

func (a *allocation) lifetime() time.Duration {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	return a._lifetime
}

func (a *allocation) setLifetime(lifetime time.Duration) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a._lifetime = lifetime
}
