module github.com/pion/turn/v3

go 1.16

require (
	github.com/pion/logging v0.2.2
	github.com/pion/randutil v0.1.0
	github.com/pion/stun/v2 v2.0.0
	github.com/pion/transport/v3 v3.0.1
	github.com/stretchr/testify v1.8.4
	golang.org/x/sys v0.15.0
)

// ebpf/xdp offload
require github.com/cilium/ebpf v0.12.3
