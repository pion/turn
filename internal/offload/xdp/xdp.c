// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

// +build ignore

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/in.h>

#include "bpf_endian.h"
#include "bpf_helpers.h"
#include "parsing_helpers.h" // taken from xdp-tutorial

#include "utils.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define MAX_MAP_ENTRIES 10240
#define MAX_UDP_SIZE 1480

struct FourTuple {
	__u32 remote_ip;
	__u32 local_ip;
	__u16 remote_port;
	__u16 local_port;
};

struct FourTupleWithChannelId {
	struct FourTuple four_tuple;
	__u32 channel_id;
};

struct FourTupleStat {
	__u64 pkts;
	__u64 bytes;
	__u64 timestamp_last;
};

enum ChanHdrAction { HDR_ADD, HDR_REMOVE };

// TURN                                TURN           Peer          Peer
// client                              server          A             B
//   |                                   |             |             |
//   |-- ChannelBind req --------------->|             |             |
//   | (Peer A to 0x4001)                |             |             |
//   |                                   |             |             |
//   |<---------- ChannelBind succ resp -|             |             |
//   |                                   |             |             |
//   |-- (0x4001) data ----------------->|             |             |
//   |                                   |=== data ===>|             |
//   |                                   |             |             |
//   |                                   |<== data ====|             |
//   |<------------------ (0x4001) data -|             |             |
//   |                                   |             |             |
//   |--- Send ind (Peer A)------------->|             |             |
//   |                                   |=== data ===>|             |
//   |                                   |             |             |
//   |                                   |<== data ====|             |
//   |<------------------ (0x4001) data -|             |             |
//   |                                   |             |             |
// RFC 8656 Figure 4

// to client
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
	__type(key, struct FourTuple);
	__type(value, struct FourTupleWithChannelId);
} turn_server_downstream_map SEC(".maps");

// to media server
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
	__type(key, struct FourTupleWithChannelId);
	__type(value, struct FourTuple);
} turn_server_upstream_map SEC(".maps");

// fourtuple stats
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
	__type(key, struct FourTuple);
	__type(value, struct FourTupleStat);
} turn_server_stats_map SEC(".maps");

// interface IP addresses
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, MAX_MAP_ENTRIES);
	__uint(pinning, LIBBPF_PIN_BY_NAME);
	__type(key, __u32);
	__type(value, __be32);
} turn_server_interface_ip_addresses_map SEC(".maps");

SEC("xdp")
int xdp_prog_func(struct xdp_md *ctx)
{
	void *data_end = (void *)(long)ctx->data_end;
	void *data = (void *)(long)ctx->data;
	int action = XDP_PASS;
	// fib lookup
	struct bpf_fib_lookup fib_params = {};
	// parsing
	int eth_type, ip_type, udp_payload_len;
	struct hdr_cursor nh;
	struct ethhdr *eth;
	struct iphdr *iphdr;
	struct udphdr *udphdr;
	__u32 *udp_payload;
	// store original values of pkt fields
	__be32 orig_saddr, orig_daddr;
	__be16 orig_udphdr_len;
	// return values
	int rc;
	long r;
	// TURN processing
	struct FourTuple *out_tuple = NULL;
	struct FourTupleStat *stat;
	struct FourTupleStat stat_new;
	enum ChanHdrAction chan_hdr_action;
	__u32 chan_data_hdr;
	__u32 chan_id;
	__u16 chan_len;
	__u16 padding;

	/* These keep track of the next header type and iterator pointer */
	nh.pos = data;

	eth_type = parse_ethhdr(&nh, data_end, &eth);
	if (eth_type < 0) {
		action = XDP_DROP;
		goto out;
	}
	if (eth_type != bpf_htons(ETH_P_IP))
		goto out;

	ip_type = parse_iphdr(&nh, data_end, &iphdr);
	if (ip_type < 0) {
		action = XDP_DROP;
		goto out;
	}
	if (ip_type != IPPROTO_UDP)
		goto out;

	udp_payload_len = parse_udphdr(&nh, data_end, &udphdr);
	if (udp_payload_len < 0) {
		action = XDP_DROP;
		goto out;
	} else if (udp_payload_len > MAX_UDP_SIZE) {
		goto out;
	}
	orig_udphdr_len = udphdr->len;

	// construct four tuple
	struct FourTuple in_tuple = {.remote_ip = iphdr->saddr,
				     .local_ip = iphdr->daddr,
				     .remote_port = udphdr->source,
				     .local_port = udphdr->dest};

	// downstream?
	struct FourTupleWithChannelId *out_tuplec_ds;
	out_tuplec_ds = bpf_map_lookup_elem(&turn_server_downstream_map, &in_tuple);
	if (likely(!out_tuplec_ds)) {
		// to overcome the situation of TURN server not knowing its local IP address:
		// try lookup '0.0.0.0'
		in_tuple.local_ip = 0;
		out_tuplec_ds = bpf_map_lookup_elem(&turn_server_downstream_map, &in_tuple);
		in_tuple.local_ip = iphdr->daddr;
	}
	if (out_tuplec_ds) {
		chan_id = out_tuplec_ds->channel_id;
		// add 4-byte space for the channel ID
		r = bpf_xdp_adjust_head(ctx, -4);
		if (r != 0)
			goto out;
		udp_payload_len += 4;
		data_end = (void *)(long)ctx->data_end;
		data = (void *)(long)ctx->data;
		// note: data_end - data is the NIC-padded length of the packet
		__u16 pkt_buf_len = data_end - data;
		udp_payload =
			data + sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct udphdr);

		// shift headers by -4 bytes (this extend UDP payload by 4 bytes)
		int bytes_left = sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct udphdr);
		int hdrs_len = bytes_left;
		while (bytes_left > 0) {
			__u8 *c = (__u8 *)data + (hdrs_len - bytes_left) + 4;
			if (c - 4 < (__u8 *)data)
				goto out;
			if (bytes_left >= 32) {
				if (c + 32 > (__u8 *)data_end)
					goto out;
				memmove(c - 4, c, 32);
				bytes_left -= 32;
			} else if (bytes_left >= 16) {
				if (c + 16 > (__u8 *)data_end)
					goto out;
				memmove(c - 4, c, 16);
				bytes_left -= 16;
			} else if (bytes_left >= 8) {
				if (c + 8 > (__u8 *)data_end)
					goto out;
				memmove(c - 4, c, 8);
				bytes_left -= 8;
			} else if (bytes_left >= 4) {
				if (c + 4 > (__u8 *)data_end)
					goto out;
				memmove(c - 4, c, 4);
				bytes_left -= 4;
			} else if (bytes_left >= 2) {
				if (c + 2 > (__u8 *)data_end)
					goto out;
				memmove(c - 4, c, 2);
				bytes_left -= 2;
			} else if (bytes_left >= 1) {
				if (c + 1 > (__u8 *)data_end)
					goto out;
				memmove(c - 4, c, 1);
				bytes_left -= 1;
			} else {
				break;
			}
		}

		// write ChannelData header with fields Channel Number and Length
		// Details: https://www.rfc-editor.org/rfc/rfc8656.html#section-12.4
		if ((__u8 *)udp_payload + 4 > (__u8 *)data_end) {
			goto out;
		}
		chan_len = (__u16)(udp_payload_len - 4);
		udp_payload[0] = bpf_htonl(((__u16)chan_id << 16) | chan_len);
		chan_data_hdr = udp_payload[0];
		chan_hdr_action = HDR_ADD;

		// add padding

		// check if new padding is necessary
		// e.g., in case of NIC-padded packets we can reuse existing padding
		int useful_len = hdrs_len + udp_payload_len;
		int existing_padding = pkt_buf_len - useful_len;

		__u16 padded_len = 4 * ((__u16)udp_payload_len / 4);
		if (padded_len < udp_payload_len) {
			padded_len += 4;
		}
		padding = padded_len - (__u16)udp_payload_len;
		udp_payload_len += padding;

		if ((existing_padding > 0) && (padding != 0)) {
			padding -= existing_padding - ((__u32)existing_padding / padding * padding);
			if (existing_padding > padding) {
				padding = 0;
			}
		}

		// add padding
		r = bpf_xdp_adjust_tail(ctx, padding);
		if (r != 0)
			goto out;

		// set out_tuple for further processing
		out_tuple = &out_tuplec_ds->four_tuple;
	} else {
		// read channel id
		udp_payload =
			data + sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct udphdr);
		if ((__u8 *)udp_payload + 4 > (__u8 *)data_end) {
			goto out;
		}
		chan_id = (bpf_ntohl(udp_payload[0]) >> 16) & 0xFFFF;
		chan_len = bpf_ntohl(udp_payload[0]); // last 16 bits only
		chan_data_hdr = udp_payload[0];
		chan_hdr_action = HDR_REMOVE;

		// upstream?
		struct FourTupleWithChannelId in_tuplec_us = {.four_tuple = in_tuple,
							      .channel_id = chan_id};
		out_tuple = bpf_map_lookup_elem(&turn_server_upstream_map, &in_tuplec_us);
		if (!out_tuple) {
			// to overcome the situation of TURN server not knowing its local IP address:
			// try lookup '0.0.0.0'
			in_tuplec_us.four_tuple.local_ip = 0;
			out_tuple = bpf_map_lookup_elem(&turn_server_upstream_map, &in_tuplec_us);
		}
		if (!out_tuple) {
			goto out;
		}

		// remove channel id
		// step1: shift the headers
		int bytes_left = sizeof(struct ethhdr) + sizeof(struct iphdr) + sizeof(struct udphdr);
		while (bytes_left > 0) {
			__u8 *c = (__u8 *)data + bytes_left;
			if (bytes_left >= 32) {
				memmove(c - 28, c - 32, 32);
				bytes_left -= 32;
			} else if (bytes_left >= 16) {
				memmove(c - 12, c - 16, 16);
				bytes_left -= 16;
			} else if (bytes_left >= 8) {
				memmove(c - 4, c - 8, 8);
				bytes_left -= 8;
			} else if (bytes_left >= 4) {
				memmove(c, c - 4, 4);
				bytes_left -= 4;
			} else if (bytes_left >= 2) {
				memmove(c + 2, c - 2, 2);
				bytes_left -= 2;
			} else if (bytes_left >= 1) {
				memmove(c + 1, c - 3, 1);
				bytes_left -= 1;
			} else {
				break;
			}
		}

		// step2: trim packet
		r = bpf_xdp_adjust_head(ctx, 4);
		if (r != 0)
			goto out;
		udp_payload_len -= 4;

		// remove padding
		padding = (__u16)udp_payload_len - chan_len;
		if (padding >= 0 && padding <= 3) {
			r = bpf_xdp_adjust_tail(ctx, -padding);
			if (r != 0)
				goto out;
			udp_payload_len -= padding;
		} else {
			goto out;
		}
	}

	// Update header fields

	// reparse headers to please the verifier
	data_end = (void *)(long)ctx->data_end;
	data = (void *)(long)ctx->data;
	nh.pos = data;
	eth_type = parse_ethhdr(&nh, data_end, &eth);
	if (eth_type != bpf_htons(ETH_P_IP)) {
		action = XDP_DROP;
		goto out;
	}
	ip_type = parse_iphdr(&nh, data_end, &iphdr);
	if (ip_type != IPPROTO_UDP) {
		action = XDP_DROP;
		goto out;
	}
	int orig_udp_data_len = parse_udphdr(&nh, data_end, &udphdr);
	if (orig_udp_data_len < 0) {
		action = XDP_DROP;
		goto out;
	} else if (orig_udp_data_len > MAX_UDP_SIZE) {
		action = XDP_DROP;
		goto out;
	}

	// udp_payload_len contains the padded UDP data,
	// orig_udp_data_len is the Data length of the incoming UDP packet
	short len_diff = udp_payload_len - orig_udp_data_len;
	// update IP len: payload + header size
	iphdr->tot_len = bpf_htons(bpf_ntohs(iphdr->tot_len) + len_diff);
	// update UDP len: payload (data and padding) changes + header size
	udphdr->len = bpf_htons(bpf_ntohs(udphdr->len) + len_diff);

	// update IP addresses
	orig_saddr = iphdr->saddr;
	orig_daddr = iphdr->daddr;
	iphdr->saddr = out_tuple->local_ip;
	iphdr->daddr = out_tuple->remote_ip;
	iphdr->check = 0;
	__u64 ip_csum = 0;
	ipv4_csum(iphdr, sizeof(*iphdr), &ip_csum);
	iphdr->check = ip_csum;

	// update UDP ports and checksum
	udphdr->source = out_tuple->local_port;
	udphdr->dest = out_tuple->remote_port;
	udphdr->check = update_udp_checksum(udphdr->check, in_tuple.local_port, udphdr->source);
	udphdr->check = update_udp_checksum(udphdr->check, in_tuple.remote_port, udphdr->dest);

	udphdr->check = update_udp_checksum(udphdr->check, orig_saddr, iphdr->saddr);
	udphdr->check = update_udp_checksum(udphdr->check, orig_daddr, iphdr->daddr);

	/* Note: we have to account two changes:
	    1 - update of the len field
	    2 - addition of new \0 blocks (e.g., padding and chan data)

	   To demo this phenomenon with Scapy:
	    pkt1 = IP()/UDP()/Raw("a")
	    pkt2 = IP()/UDP()/Raw("a\0")
	    pkt1.show2()
	    pkt2.show2()
	    Checksums:
	     - pkt1: 0xa06f
	     - pkt2: 0xa06d
	     diff: 2
	*/
	udphdr->check = update_udp_checksum(udphdr->check, orig_udphdr_len, udphdr->len);
	udphdr->check = update_udp_checksum(udphdr->check, orig_udphdr_len, udphdr->len);

	switch (chan_hdr_action) {
	case HDR_ADD:
		udphdr->check = update_udp_checksum(udphdr->check, 0, chan_data_hdr);
		break;
	case HDR_REMOVE:
		udphdr->check = update_udp_checksum(udphdr->check, chan_data_hdr, 0);
		break;
	default:
		// something has really really gone wrong
		action = XDP_DROP;
		goto out;
		break;
	}

	// Send packet
	fib_params.family = AF_INET;
	fib_params.tos = iphdr->tos;
	fib_params.l4_protocol = iphdr->protocol;
	fib_params.sport = 0;
	fib_params.dport = 0;
	fib_params.tot_len = bpf_ntohs(iphdr->tot_len);
	fib_params.ipv4_src = iphdr->saddr;
	fib_params.ipv4_dst = iphdr->daddr;

	fib_params.ifindex = ctx->ingress_ifindex;

	rc = bpf_fib_lookup(ctx, &fib_params, sizeof(fib_params), 0);
	switch (rc) {
	case BPF_FIB_LKUP_RET_SUCCESS: /* lookup successful */
		// set eth addrs
		memcpy(eth->h_dest, fib_params.dmac, ETH_ALEN);
		memcpy(eth->h_source, fib_params.smac, ETH_ALEN);

		// update IP source address with the interface's address
		orig_saddr = iphdr->saddr;
		__be32 *new_saddr;
		new_saddr = bpf_map_lookup_elem(&turn_server_interface_ip_addresses_map,
						&fib_params.ifindex);
		if (!new_saddr) {
			action = XDP_DROP;
			goto out;
		}
		iphdr->saddr = *new_saddr;

		// update ip and udp checksums
		iphdr->check = update_udp_checksum(iphdr->check, orig_saddr, iphdr->saddr);
		udphdr->check = update_udp_checksum(udphdr->check, orig_saddr, iphdr->saddr);

		// redirect packet
		action = bpf_redirect(fib_params.ifindex, 0);
		break;

	case BPF_FIB_LKUP_RET_BLACKHOLE:   /* dest is blackholed; can be dropped */
	case BPF_FIB_LKUP_RET_UNREACHABLE: /* dest is unreachable; can be dropped */
	case BPF_FIB_LKUP_RET_PROHIBIT:	   /* dest not allowed; can be dropped */
		action = XDP_DROP;
		break;

	case BPF_FIB_LKUP_RET_NOT_FWDED:    /* packet is not forwarded */
	case BPF_FIB_LKUP_RET_FWD_DISABLED: /* fwding is not enabled on ingress */
	case BPF_FIB_LKUP_RET_UNSUPP_LWT:   /* fwd requires encapsulation */
	case BPF_FIB_LKUP_RET_NO_NEIGH:	    /* no neighbor entry for nh */
	case BPF_FIB_LKUP_RET_FRAG_NEEDED:  /* fragmentation required to fwd */
		break;
	}

	// Account sent packet
	if (((action == XDP_PASS) || (action == XDP_REDIRECT))) {
		stat = bpf_map_lookup_elem(&turn_server_stats_map, out_tuple);
		__u64 bytes = data_end - data;
		__u64 ts = bpf_ktime_get_ns();
		if (stat) {
			stat->pkts += 1;
			stat->bytes += bytes;
			stat->timestamp_last = ts;
		} else {
			stat_new.pkts = 1;
			stat_new.bytes = bytes;
			stat_new.timestamp_last = ts;
			bpf_map_update_elem(&turn_server_stats_map, out_tuple, &stat_new, BPF_ANY);
		}
	}

out:
	return action;
}
