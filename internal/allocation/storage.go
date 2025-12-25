package allocation

type Storage interface {
	GetAllocation(fingerprint FiveTupleFingerprint) (*Allocation, bool)
	AddAllocation(allocation *Allocation)
	DeleteAllocation(fingerprint FiveTupleFingerprint)
	GetAllocations() map[FiveTupleFingerprint]*Allocation
	Close() error
}
