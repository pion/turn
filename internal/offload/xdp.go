// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package offload

import (
	"encoding/binary"
	"net"
	"os"
	"strconv"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/pion/logging"
	"github.com/pion/turn/v3/internal/offload/xdp"
	"github.com/pion/turn/v3/internal/proto"
)

// XdpEngine represents an XDP offload engine; implements OffloadEngine
type XdpEngine struct {
	interfaces    []net.Interface
	upstreamMap   *ebpf.Map
	downstreamMap *ebpf.Map
	ipaddrsMap    *ebpf.Map
	statsMap      *ebpf.Map
	objs          xdp.BpfObjects
	links         []link.Link
	log           logging.LeveledLogger
}

// NewXdpEngine creates an uninitialized XDP offload engine
func NewXdpEngine(ifs []net.Interface, log logging.LeveledLogger) (*XdpEngine, error) {
	if xdp.IsInitialized {
		return nil, ErrXDPAlreadyInitialized
	}
	e := &XdpEngine{
		interfaces: ifs,
		log:        log,
	}
	return e, nil
}

func (o *XdpEngine) unpinMaps() error {
	// unlink maps
	if o.downstreamMap != nil {
		if err := o.downstreamMap.Unpin(); err != nil {
			return err
		}
	}
	if o.upstreamMap != nil {
		if err := o.upstreamMap.Unpin(); err != nil {
			return err
		}
	}
	if o.statsMap != nil {
		if err := o.statsMap.Unpin(); err != nil {
			return err
		}
	}

	if o.ipaddrsMap != nil {
		if err := o.ipaddrsMap.Unpin(); err != nil {
			return err
		}
	}

	return nil
}

// Init sets up the environment for the XDP program: enables IPv4
// forwarding in the kernel; links maps of the XDP program; and,
// starts the XDP program on network interfaces.
// Based on https://github.com/l7mp/l7mp/blob/master/udp-offload.js#L232
func (o *XdpEngine) Init() error {
	if xdp.IsInitialized {
		return ErrXDPAlreadyInitialized
	}
	// enable ipv4 forwarding
	f := "/proc/sys/net/ipv4/conf/all/forwarding"
	data, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	val, err := strconv.Atoi(string(data[:len(data)-1]))
	if err != nil {
		return err
	}
	if val != 1 {
		//nolint:gosec
		if e := os.WriteFile(f, []byte("1"), 0o644); e != nil {
			return e
		}
	}

	// unlink maps if they exist
	if err = o.unpinMaps(); err != nil {
		return err
	}

	// Load pre-compiled programs into the kernel
	o.objs = xdp.BpfObjects{}
	opts := ebpf.CollectionOptions{Maps: ebpf.MapOptions{PinPath: xdp.BpfMapPinPath}}
	if err = xdp.LoadBpfObjects(&o.objs, &opts); err != nil {
		return err
	}
	o.downstreamMap = o.objs.TurnServerDownstreamMap
	o.upstreamMap = o.objs.TurnServerUpstreamMap
	o.ipaddrsMap = o.objs.TurnServerInterfaceIpAddressesMap
	o.statsMap = o.objs.TurnServerStatsMap

	ifNames := []string{}
	// Attach program to interfaces
	for _, iface := range o.interfaces {
		l, linkErr := link.AttachXDP(link.XDPOptions{
			Program:   o.objs.XdpProgFunc,
			Interface: iface.Index,
		})
		if linkErr != nil {
			return linkErr
		}
		o.links = append(o.links, l)
		ifNames = append(ifNames, iface.Name)
	}

	// populate interface IP addresses map
	ifIPMap, err := collectInterfaceIpv4Addrs()
	if err != nil {
		return err
	}
	for idx, addr := range ifIPMap {
		err := o.ipaddrsMap.Put(uint32(idx),
			binary.LittleEndian.Uint32(addr))
		if err != nil {
			return err
		}
	}

	xdp.IsInitialized = true

	o.log.Infof("Init done on interfaces: %s", ifNames)
	return nil
}

// Shutdown stops the XDP offloading engine
func (o *XdpEngine) Shutdown() {
	if !xdp.IsInitialized {
		return
	}

	// close objects
	if err := o.objs.Close(); err != nil {
		o.log.Errorf("Error during shutdown: %s", err.Error())
		return
	}

	// close links
	for _, l := range o.links {
		if err := l.Close(); err != nil {
			o.log.Errorf("Error during shutdown: %s", err.Error())
			return
		}
	}

	// unlink maps
	if err := o.unpinMaps(); err != nil {
		o.log.Errorf("Error during shutdown: %s", err.Error())
		return
	}

	xdp.IsInitialized = false

	o.log.Info("Shutdown done")
}

// Upsert creates a new XDP offload between a client and a peer
func (o *XdpEngine) Upsert(client, peer Connection) error {
	err := o.validate(client, peer)
	if err != nil {
		return err
	}
	p, err := bpfFourTuple(peer)
	if err != nil {
		return err
	}
	cft, err := bpfFourTuple(client)
	if err != nil {
		return err
	}
	c := xdp.BpfFourTupleWithChannelId{
		FourTuple: *cft,
		ChannelId: client.ChannelID,
	}

	if err := o.downstreamMap.Put(p, c); err != nil {
		o.log.Errorf("Error in upsert (downstream map): %s", err.Error())
		return err
	}
	if err := o.upstreamMap.Put(c, p); err != nil {
		o.log.Errorf("Error in upsert (upstream map): %s", err.Error())
		return err
	}

	// register with Local IP = 0 to support multi NIC setups
	p.LocalIp = 0
	if err := o.downstreamMap.Put(p, c); err != nil {
		o.log.Errorf("Error in upsert (downstream map): %s", err.Error())
		return err
	}

	o.log.Infof("Create offload between client: %+v and peer: %+v", client, peer)
	return nil
}

// Remove removes an XDP offload between a client and a peer
func (o *XdpEngine) Remove(client, peer Connection) error {
	p, err := bpfFourTuple(peer)
	if err != nil {
		return err
	}
	cft, err := bpfFourTuple(client)
	if err != nil {
		return err
	}
	c := xdp.BpfFourTupleWithChannelId{
		FourTuple: *cft,
		ChannelId: client.ChannelID,
	}

	if err := o.downstreamMap.Delete(p); err != nil {
		return err
	}

	if err := o.upstreamMap.Delete(c); err != nil {
		return err
	}

	p.LocalIp = 0
	if err := o.downstreamMap.Delete(p); err != nil {
		return err
	}

	o.log.Infof("Remove offload between client: %+v and peer: %+v", client, peer)
	return nil
}

// List returns all upstream/downstream offloads stored in the corresponding eBPF maps
func (o *XdpEngine) List() (map[Connection]Connection, error) {
	var p xdp.BpfFourTuple
	var c xdp.BpfFourTupleWithChannelId

	r := make(map[Connection]Connection)

	iterD := o.downstreamMap.Iterate()
	for iterD.Next(&p, &c) {
		k := Connection{
			RemoteAddr: &net.UDPAddr{IP: ipv4(p.RemoteIp), Port: int(p.RemotePort)},
			LocalAddr:  &net.UDPAddr{IP: ipv4(p.LocalIp), Port: int(p.LocalPort)},
			Protocol:   proto.ProtoUDP,
		}
		v := Connection{
			RemoteAddr: &net.UDPAddr{
				IP:   ipv4(c.FourTuple.RemoteIp),
				Port: int(c.FourTuple.RemotePort),
			},
			LocalAddr: &net.UDPAddr{
				IP:   ipv4(c.FourTuple.LocalIp),
				Port: int(c.FourTuple.LocalPort),
			},
			Protocol:  proto.ProtoUDP,
			ChannelID: c.ChannelId,
		}
		r[k] = v
	}
	if err := iterD.Err(); err != nil {
		return nil, err
	}

	iterU := o.upstreamMap.Iterate()
	for iterU.Next(&c, &p) {
		k := Connection{
			RemoteAddr: &net.UDPAddr{
				IP:   ipv4(c.FourTuple.RemoteIp),
				Port: int(c.FourTuple.RemotePort),
			},
			LocalAddr: &net.UDPAddr{
				IP:   ipv4(c.FourTuple.LocalIp),
				Port: int(c.FourTuple.LocalPort),
			},
			Protocol:  proto.ProtoUDP,
			ChannelID: c.ChannelId,
		}
		v := Connection{
			RemoteAddr: &net.UDPAddr{IP: ipv4(p.RemoteIp), Port: int(p.RemotePort)},
			LocalAddr:  &net.UDPAddr{IP: ipv4(p.LocalIp), Port: int(p.LocalPort)},
			Protocol:   proto.ProtoUDP,
		}
		r[k] = v
	}
	if err := iterU.Err(); err != nil {
		return nil, err
	}

	return r, nil
}

// validate checks the eligibility of an offload between a client and a peer
func (o *XdpEngine) validate(client, peer Connection) error {
	// check UDP
	if (client.Protocol != proto.ProtoUDP) || (peer.Protocol != proto.ProtoUDP) {
		err := ErrUnsupportedProtocol
		o.log.Warn(err.Error())
		return err
	}
	// check this is not a local redirect
	p, ok := peer.RemoteAddr.(*net.UDPAddr)
	if !ok {
		err := ErrUnsupportedProtocol
		o.log.Warn(err.Error())
		return err
	}
	localNet := net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)}
	ifAddrs, err := collectInterfaceIpv4Addrs()
	if err != nil {
		o.log.Warn(err.Error())
		return err
	}
	for _, ip := range ifAddrs {
		if (p.IP.Equal(ip)) && (!localNet.Contains(p.IP)) {
			err := ErrXDPLocalRedirectProhibited
			o.log.Warn(err.Error())
			return err
		}
	}

	return nil
}

// collectInterfaceIpv4Addrs creates a map of interface ids to interface IPv4 addresses
func collectInterfaceIpv4Addrs() (map[int]net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	m := make(map[int]net.IP)
	for _, netIf := range ifs {
		addrs, err := netIf.Addrs()
		if err == nil && len(addrs) > 0 {
			n, ok := addrs[0].(*net.IPNet)
			if !ok {
				continue
			}
			if addr := n.IP.To4(); addr != nil {
				m[netIf.Index] = addr
			}
		}
	}
	return m, nil
}

func hostToNetShort(i uint16) uint16 {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, i)
	return binary.BigEndian.Uint16(b)
}

func ipv4(i uint32) net.IP {
	ip := make(net.IP, 4)
	binary.LittleEndian.PutUint32(ip, i)
	return ip
}

// bpfFourTuple creates an xdp.BpfFourTuple struct that can be used in the XDP offload maps
func bpfFourTuple(c Connection) (*xdp.BpfFourTuple, error) {
	if c.Protocol != proto.ProtoUDP {
		return nil, ErrUnsupportedProtocol
	}
	l, lok := c.LocalAddr.(*net.UDPAddr)
	r, rok := c.RemoteAddr.(*net.UDPAddr)
	if !lok || !rok {
		return nil, ErrUnsupportedProtocol
	}
	var localIP uint32
	if l.IP.To4() != nil {
		localIP = binary.LittleEndian.Uint32(l.IP.To4())
	}
	var remoteIP uint32
	if r.IP.To4() != nil {
		remoteIP = binary.LittleEndian.Uint32(r.IP.To4())
	}

	t := xdp.BpfFourTuple{
		RemoteIp:   remoteIP,
		LocalIp:    localIP,
		RemotePort: hostToNetShort(uint16(r.Port)),
		LocalPort:  hostToNetShort(uint16(l.Port)),
	}
	return &t, nil
}
