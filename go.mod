module github.com/pion/turn/v3

go 1.21.0

require (
	github.com/pion/logging v0.2.2
	github.com/pion/randutil v0.1.0
	github.com/pion/stun/v2 v2.0.0
	github.com/pion/transport/v3 v3.0.5
	github.com/stretchr/testify v1.9.0
	golang.org/x/sys v0.22.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pion/dtls/v2 v2.2.7 // indirect
	github.com/pion/transport/v2 v2.2.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/wlynxg/anet v0.0.3 // indirect
	golang.org/x/crypto v0.21.0 // indirect
	golang.org/x/exp v0.0.0-20230224173230-c95f2b4c22f2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// ebpf/xdp offload
require github.com/cilium/ebpf v0.15.0
