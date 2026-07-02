package network

import (
	"net"
	"time"

	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// IPv6RouteEntry is an IPv6 routing table row.
type IPv6RouteEntry struct {
	Network   *net.IPNet
	NextHop   pdu.IPv6Address
	Interface string
	Metric    int
	Protocol  protocol.RouteProtocol
}

// IPv6NeighborCache maps IPv6 to MAC (NDP).
type IPv6NeighborCache struct {
	MAC       pdu.MACAddress
	ExpiresAt time.Duration
}

// AddIPv6Interface assigns IPv6 to a router port.
func (r *Router) AddIPv6Interface(portID string, ip pdu.IPv6Address, prefixLen int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IPv6Interfaces == nil {
		r.IPv6Interfaces = make(map[string]pdu.IPv6Address)
	}
	if r.IPv6Prefixes == nil {
		r.IPv6Prefixes = make(map[string]int)
	}
	r.IPv6Interfaces[portID] = ip
	r.IPv6Prefixes[portID] = prefixLen

	ipNet := ipv6CIDR(ip, prefixLen)
	r.IPv6Routes = append(r.IPv6Routes, IPv6RouteEntry{
		Network: ipNet, Interface: portID, Protocol: protocol.RouteConnected,
	})
}

func ipv6CIDR(ip pdu.IPv6Address, prefixLen int) *net.IPNet {
	ipAddr := net.ParseIP(string(ip))
	if ipAddr == nil {
		return nil
	}
	mask := net.CIDRMask(prefixLen, 128)
	return &net.IPNet{IP: ipAddr.Mask(mask), Mask: mask}
}

// MatchIPv6Route performs longest-prefix match for IPv6.
func (r *Router) MatchIPv6Route(dest pdu.IPv6Address) (*IPv6RouteEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	target := net.ParseIP(string(dest))
	var best *IPv6RouteEntry
	bestLen := -1
	for i := range r.IPv6Routes {
		entry := &r.IPv6Routes[i]
		if entry.Network.Contains(target) {
			ones, _ := entry.Network.Mask.Size()
			if ones > bestLen {
				bestLen = ones
				best = entry
			}
		}
	}
	return best, best != nil
}

// OwnsIPv6 returns true if dest is assigned to this router.
func (r *Router) OwnsIPv6(ip pdu.IPv6Address) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ifaceIP := range r.IPv6Interfaces {
		if ifaceIP == ip {
			return true
		}
	}
	return false
}

// LookupNDP returns cached neighbor MAC for IPv6.
func (r *Router) LookupNDP(ip pdu.IPv6Address, simTime time.Duration) (pdu.MACAddress, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.NDPCache == nil {
		return "", false
	}
	e, ok := r.NDPCache[ip]
	if !ok || simTime > e.ExpiresAt {
		return "", false
	}
	return e.MAC, true
}

// LearnNDP records IPv6 neighbor.
func (r *Router) LearnNDP(ip pdu.IPv6Address, mac pdu.MACAddress, simTime time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.NDPCache == nil {
		r.NDPCache = make(map[pdu.IPv6Address]IPv6NeighborCache)
	}
	r.NDPCache[ip] = IPv6NeighborCache{MAC: mac, ExpiresAt: simTime + 20*time.Minute}
}

// QueueIPv6Packet buffers IPv6 packet pending NDP.
func (r *Router) QueueIPv6Packet(nextHop pdu.IPv6Address, pkt *pdu.IPv6Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IPv6Queue == nil {
		r.IPv6Queue = make(map[pdu.IPv6Address][]*pdu.IPv6Packet)
	}
	r.IPv6Queue[nextHop] = append(r.IPv6Queue[nextHop], pkt)
}

// DequeueIPv6 returns queued packets for a resolved neighbor.
func (r *Router) DequeueIPv6(nextHop pdu.IPv6Address) []*pdu.IPv6Packet {
	r.mu.Lock()
	defer r.mu.Unlock()
	pkts := r.IPv6Queue[nextHop]
	delete(r.IPv6Queue, nextHop)
	return pkts
}

// BuildNDPSolicit creates neighbor solicitation frame.
func (r *Router) BuildNDPSolicit(portID string, target pdu.IPv6Address) *pdu.WireFrame {
	r.mu.RLock()
	defer r.mu.RUnlock()
	srcIP := r.IPv6Interfaces[portID]
	srcMAC := r.InterfaceMAC[portID]
	ndp := &pdu.NDPPacket{Type: pdu.NDPNeighborSolicit, TargetIP: target, SenderIP: srcIP, SenderMAC: srcMAC}
	frame, _ := pdu.NewNDPFrame(pdu.MACIPv6Multicast, srcMAC, ndp)
	return &pdu.WireFrame{Frame: frame, Physical: &pdu.L1Metadata{SourceNodeID: r.ID, SourcePortID: portID}}
}

// HandleNDP processes NDP messages.
func (r *Router) HandleNDP(portID string, ndp *pdu.NDPPacket, simTime time.Duration) (*pdu.WireFrame, []*pdu.IPv6Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ndp.SenderIP != "" && ndp.SenderMAC != "" {
		if r.NDPCache == nil {
			r.NDPCache = make(map[pdu.IPv6Address]IPv6NeighborCache)
		}
		r.NDPCache[ndp.SenderIP] = IPv6NeighborCache{MAC: ndp.SenderMAC, ExpiresAt: simTime + 20*time.Minute}
	}

	if ndp.Type == pdu.NDPNeighborSolicit {
		for ip, ifaceIP := range r.IPv6Interfaces {
			if ifaceIP == ndp.TargetIP && ip == portID {
				reply := &pdu.NDPPacket{
					Type: pdu.NDPNeighborAdvert, TargetIP: ndp.TargetIP,
					SenderIP: ifaceIP, SenderMAC: r.InterfaceMAC[portID],
					TargetMAC: ndp.SenderMAC,
				}
				frame, _ := pdu.NewNDPFrame(ndp.SenderMAC, r.InterfaceMAC[portID], reply)
				return &pdu.WireFrame{Frame: frame, Physical: &pdu.L1Metadata{SourceNodeID: r.ID, SourcePortID: portID}}, nil
			}
		}
	}

	if ndp.Type == pdu.NDPNeighborAdvert {
		waiting := r.IPv6Queue[ndp.SenderIP]
		delete(r.IPv6Queue, ndp.SenderIP)
		return nil, waiting
	}
	return nil, nil
}

// FormatIPv6RouteTable returns JSON-friendly IPv6 routes.
func (r *Router) FormatIPv6RouteTable() []RouteTableRow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rows := make([]RouteTableRow, 0, len(r.IPv6Routes))
	for _, e := range r.IPv6Routes {
		nh := string(e.NextHop)
		if e.Protocol == protocol.RouteConnected {
			nh = "directly connected"
		}
		rows = append(rows, RouteTableRow{
			Protocol: string(e.Protocol), Network: e.Network.String(),
			Metric: e.Metric, NextHop: nh, Interface: e.Interface,
		})
	}
	return rows
}
