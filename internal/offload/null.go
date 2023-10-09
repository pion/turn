// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package offload

import (
	"github.com/pion/logging"
)

// NullEngine is a null offload engine
type NullEngine struct {
	conntrack map[Connection]Connection
	log       logging.LeveledLogger
}

// NewNullEngine creates an uninitialized null offload engine
func NewNullEngine(log logging.LeveledLogger) (*NullEngine, error) {
	c := make(map[Connection]Connection)
	return &NullEngine{conntrack: c, log: log}, nil
}

// Init initializes the Null engine
func (o *NullEngine) Init() error {
	o.log.Info("(NullOffload) Init done")
	return nil
}

// Shutdown stops the null offloading engine
func (o *NullEngine) Shutdown() {
	if o.log == nil {
		return
	}
	o.log.Info("(NullOffload) Shutdown done")
}

// Upsert imitates an offload creation between a client and a peer
func (o *NullEngine) Upsert(client, peer Connection) error {
	o.log.Debugf("Would create offload between client: %+v and peer: %+v", client, peer)
	o.conntrack[client] = peer
	return nil
}

// Remove imitates offload deletion between a client and a peer
func (o *NullEngine) Remove(client, peer Connection) error {
	o.log.Debugf("Would remove offload between client: %+v and peer: %+v", client, peer)

	if _, ok := o.conntrack[client]; !ok {
		return ErrConnectionNotFound
	}
	delete(o.conntrack, client)

	return nil
}

// List returns the internal conntrack map, which keeps track of all
// the connections through the proxy
func (o *NullEngine) List() (map[Connection]Connection, error) {
	r := make(map[Connection]Connection)
	for k, v := range o.conntrack {
		r[k] = v
	}

	return r, nil
}
