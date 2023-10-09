// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// +build ignore

#ifndef __XDP_TURN_OFFLOAD_UTILS__
#define __XDP_TURN_OFFLOAD_UTILS__


#ifndef memcpy
#define memcpy(dest, src, n) __builtin_memcpy((dest), (src), (n))
#endif
#ifndef memmove
#define memmove(dest, src, n) __builtin_memmove((dest), (src), (n))
#endif

#ifndef likely
#define likely(x)       __builtin_expect((x),1)
#endif
#ifndef unlikely
#define unlikely(x)     __builtin_expect((x),0)
#endif
/* from katran/lib/bpf/csum_helpers.h */
__attribute__((__always_inline__)) static inline __u16 csum_fold_helper(__u64 csum)
{
	int i;
#pragma unroll
	for (i = 0; i < 4; i++) {
		if (csum >> 16)
			csum = (csum & 0xffff) + (csum >> 16);
	}
	return ~csum;
}

/* from katran/lib/bpf/csum_helpers.h */
__attribute__((__always_inline__)) static inline void ipv4_csum(void *data_start, int data_size,
								__u64 *csum)
{
	*csum = bpf_csum_diff(0, 0, data_start, data_size, *csum);
	*csum = csum_fold_helper(*csum);
}

/* from AirVantage/sbulb/sbulb/bpf/checksum.c */
// Update checksum following RFC 1624 (Eqn. 3):
// https://tools.ietf.org/html/rfc1624
//     HC' = ~(~HC + ~m + m')
// Where :
//   HC	 - old checksum in header
//   HC' - new checksum in header
//   m	 - old value
//   m'	 - new value
__attribute__((__always_inline__)) static inline void update_csum(__u64 *csum, __be32 old_addr,
								  __be32 new_addr)
{
	// ~HC
	*csum = ~*csum;
	*csum = *csum & 0xffff;
	// + ~m
	__u32 tmp;
	tmp = ~old_addr;
	*csum += tmp;
	// + m
	*csum += new_addr;
	// then fold and complement result !
	*csum = csum_fold_helper(*csum);
}

/* from AirVantage/sbulb/sbulb/bpf/ipv4.c */
__attribute__((__always_inline__)) static inline int update_udp_checksum(__u64 cs, int old_addr,
									 int new_addr)
{
	update_csum(&cs, old_addr, new_addr);
	return cs;
}

#endif  /* __XDP_TURN_OFFLOAD_UTILS__ */
