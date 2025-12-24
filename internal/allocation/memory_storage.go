// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package allocation

import (
	"sync"
)

type MemoryStorage struct {
	allocations map[FiveTupleFingerprint]*Allocation
	lock        sync.RWMutex
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		allocations: make(map[FiveTupleFingerprint]*Allocation),
	}
}

func (s *MemoryStorage) GetAllocation(fingerprint FiveTupleFingerprint) (*Allocation, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	alloc, ok := s.allocations[fingerprint]
	return alloc, ok
}

func (s *MemoryStorage) AddAllocation(alloc *Allocation) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.allocations[alloc.fiveTuple.Fingerprint()] = alloc
}

func (s *MemoryStorage) DeleteAllocation(fingerprint FiveTupleFingerprint) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.allocations, fingerprint)
}

func (s *MemoryStorage) GetAllocations() map[FiveTupleFingerprint]*Allocation {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return s.allocations
}

func (s *MemoryStorage) Close() error {
	return nil
}
