// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"bytes"
	"context"
	"encoding/gob"
	"sync"

	"github.com/go-redis/redis/v8"
)

type RedisStorage struct {
	client *redis.Client
	lock   sync.RWMutex
}

func NewRedisStorage(client *redis.Client) *RedisStorage {
	return &RedisStorage{
		client: client,
	}
}

/**
* GetAllocation returns an allocation from Redis
**/
func (s *RedisStorage) GetAllocation(fingerprint FiveTupleFingerprint) (*Allocation, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(fingerprint); err != nil {
		return nil, false
	}

	val, err := s.client.Get(context.Background(), buf.String()).Result()
	if err != nil {
		return nil, false
	}

	alloc := &Allocation{}
	if err := alloc.UnmarshalBinary([]byte(val)); err != nil {
		return nil, false
	}

	return alloc, true
}

/**
*  AddAllocation adds an allocation to Redis
**/
func (s *RedisStorage) AddAllocation(alloc *Allocation) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var bufKey bytes.Buffer
	encKey := gob.NewEncoder(&bufKey)
	if err := encKey.Encode(alloc.fiveTuple.Fingerprint()); err != nil {
		return
	}

	val, err := alloc.MarshalBinary()
	if err != nil {
		return
	}

	s.client.Set(context.Background(), bufKey.String(), val, 0)
}

/**
*DeleteAllocation removes an allocation from Redis.
**/
func (s *RedisStorage) DeleteAllocation(fingerprint FiveTupleFingerprint) {
	s.lock.Lock()
	defer s.lock.Unlock()

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(fingerprint); err != nil {
		return
	}

	s.client.Del(context.Background(), buf.String())
}

/**
*GetAllocations returns all allocations from Redis.
**/
func (s *RedisStorage) GetAllocations() map[FiveTupleFingerprint]*Allocation {
	s.lock.RLock()
	defer s.lock.RUnlock()

	allocations := make(map[FiveTupleFingerprint]*Allocation)
	keys, err := s.client.Keys(context.Background(), "*").Result()
	if err != nil {
		return allocations
	}

	for _, key := range keys {
		val, err := s.client.Get(context.Background(), key).Result()
		if err != nil {
			continue
		}

		alloc := &Allocation{}
		if err := alloc.UnmarshalBinary([]byte(val)); err != nil {
			continue
		}
		allocations[alloc.fiveTuple.Fingerprint()] = alloc
	}

	return allocations
}

/**
*Close closes the Redis client.
**/
func (s *RedisStorage) Close() error {
	return s.client.Close()
}
