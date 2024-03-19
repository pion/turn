# SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

GO=go
CLANG_FORMAT=clang-format

default: build

build: generate

fetch-libbpf-headers:
	@if ! find internal/offload/xdp/headers/bpf_* >/dev/null 2>&1; then\
		 cd internal/offload/xdp/headers && \
		./fetch-libbpf-headers.sh;\
	fi

generate: fetch-libbpf-headers
	cd internal/offload/xdp/ && \
	$(GO) generate

format-offload:
	$(CLANG_FORMAT) -i --style=file internal/offload/xdp/xdp.c

clean-offload:
	rm -vf internal/offload/xdp/bpf_bpfe*.o
	rm -vf internal/offload/xdp/bpf_bpfe*.go

purge-offload: clean-offload
	rm -vf internal/offload/xdp/headers/bpf_*

test:
	go test -v

bench: build
	go test -bench=.
