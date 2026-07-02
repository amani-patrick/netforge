package network

import (
	"fmt"
	"sync"
	"time"

	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// ArpEntry stores mapped hardware addresses with sim-time expiration.
type ArpEntry struct {
	MAC       pdu.MACAddress
	ExpiresAt time.Duration
}

// PortNeighbor maps a local port to its remote router endpoint.
type PortNeighbor struct {
	RouterID string
	PortID   string
	RemoteIP pdu.IPAddress
}

// Router represents a virtual Layer 3 device running a full network stack.
type Router struct {
	ID            string
	Interfaces    map[string]pdu.IPAddress
	InterfaceMAC  map[string]pdu.MACAddress
	InterfaceMask map[string]string
	RoutingTable  []RouteEntry
	ArpCache      map[pdu.IPAddress]ArpEntry
	PacketQueue   map[pdu.IPAddress][]*pdu.IPv4Packet
	PortNeighbors map[string]PortNeighbor
	Ospf          *protocol.OspfDaemon
	Rip           *protocol.RipDaemon
	Eigrp         *protocol.EigrpDaemon
	Bgp           *protocol.BgpDaemon
	IPv6Interfaces map[string]pdu.IPv6Address
	IPv6Prefixes   map[string]int
	IPv6Routes     []IPv6RouteEntry
	NDPCache       map[pdu.IPv6Address]IPv6NeighborCache
	IPv6Queue      map[pdu.IPv6Address][]*pdu.IPv6Packet
	ACLs          map[string]*ACL
	IfacePolicies map[string]*IfacePolicy
	NAT           *NATTable
	DHCPPools     map[string]*DHCPPool
	DHCPExcluded  []string
	DNS           *DNSServerDB
	CDPNeighbors  []CDPNeighbor
	ClassMaps     map[string]*QoSClassMap
	PolicyMaps    map[string]*QoSPolicyMap
	IfaceQoS      map[string]*QoSServicePolicy
	qosPolice     map[string]map[string]*QoSPoliceBucket
	Hostname      string
	HSRPGroups    map[string]*HSRPGroup
	ISAKMPPolicies map[int]*ISAKMPPolicy
	TransformSets map[string]*IPSecTransformSet
	CryptoMaps    map[string][]CryptoMapEntry
	IfaceCryptoMap map[string]string
	PreSharedKeys map[string]string
	IKEPeers      map[string]*IKEPeerState
	mu            sync.RWMutex
}

// NewRouter instantiates a clean virtual router with initialization maps.
func NewRouter(id string) *Router {
	return &Router{
		ID:            id,
		Interfaces:    make(map[string]pdu.IPAddress),
		InterfaceMAC:  make(map[string]pdu.MACAddress),
		InterfaceMask: make(map[string]string),
		RoutingTable:  make([]RouteEntry, 0),
		ArpCache:      make(map[pdu.IPAddress]ArpEntry),
		PacketQueue:   make(map[pdu.IPAddress][]*pdu.IPv4Packet),
		PortNeighbors: make(map[string]PortNeighbor),
	}
}

// AddInterface assigns an IP address and mask to a router port.
func (r *Router) AddInterface(portID string, ip pdu.IPAddress, mask string, mac pdu.MACAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Interfaces[portID] = ip
	r.InterfaceMask[portID] = mask
	r.InterfaceMAC[portID] = mac

	cidr := ipToCIDR(ip, mask)
	_ = r.addRouteLocked(cidr, pdu.IPAddress(""), portID, 0, protocol.RouteConnected, protocol.AdminDistConnected)
}

// AssignLinkAddress sets a /30 point-to-point address on an interface for inter-router links.
func (r *Router) AssignLinkAddress(portID string, linkIndex int, isFirst bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	octet2 := (linkIndex * 4) / 256
	octet3 := (linkIndex * 4) % 256
	host := 1
	if !isFirst {
		host = 2
	}
	ip := pdu.IPAddress(fmt.Sprintf("10.%d.%d.%d", octet2, octet3, host))
	mask := "255.255.255.252"

	if _, exists := r.Interfaces[portID]; exists {
		return
	}

	macOctet := (linkIndex % 254) + 1
	mac := pdu.MACAddress(fmt.Sprintf("00:1A:2B:%02X:%02X:%02X", octet2, octet3, macOctet))

	r.Interfaces[portID] = ip
	r.InterfaceMask[portID] = mask
	r.InterfaceMAC[portID] = mac

	cidr := ipToCIDR(ip, mask)
	_ = r.addRouteLocked(cidr, pdu.IPAddress(""), portID, 0, protocol.RouteConnected, protocol.AdminDistConnected)
}

// SetNeighbor records the remote router connected to a local port.
func (r *Router) SetNeighbor(localPort, remoteRouterID, remotePort string, remoteIP pdu.IPAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.PortNeighbors[localPort] = PortNeighbor{RouterID: remoteRouterID, PortID: remotePort, RemoteIP: remoteIP}
}

// OwnsIP returns true if the IP is assigned to one of the router interfaces.
func (r *Router) OwnsIP(ip pdu.IPAddress) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, ifaceIP := range r.Interfaces {
		if ifaceIP == ip {
			return true
		}
	}
	for _, g := range r.HSRPGroups {
		if g.VirtualIP == ip && g.State == HSRPActive {
			return true
		}
	}
	return false
}

// LookupARP returns a cached MAC valid at simTime.
func (r *Router) LookupARP(ip pdu.IPAddress, simTime time.Duration) (pdu.MACAddress, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.ArpCache[ip]
	if !ok || simTime > entry.ExpiresAt {
		return "", false
	}
	return entry.MAC, true
}

// QueuePacket buffers an IP packet waiting for ARP resolution.
func (r *Router) QueuePacket(nextHop pdu.IPAddress, pkt *pdu.IPv4Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.PacketQueue[nextHop] = append(r.PacketQueue[nextHop], pkt)
}

// BuildARPRequest creates a broadcast ARP request frame for a target IP.
func (r *Router) BuildARPRequest(portID string, targetIP pdu.IPAddress) *pdu.WireFrame {
	r.mu.RLock()
	defer r.mu.RUnlock()

	senderIP, ok := r.Interfaces[portID]
	if !ok {
		return nil
	}
	senderMAC := r.InterfaceMAC[portID]

	arp := &pdu.ARPPacket{
		HardwareType: 1,
		ProtocolType: 0x0800,
		Operation:    pdu.ArpRequest,
		SenderMAC:    senderMAC,
		SenderIP:     senderIP,
		TargetMAC:    pdu.MACBroadcast,
		TargetIP:     targetIP,
	}
	frame, err := pdu.NewARPFrame(pdu.MACBroadcast, senderMAC, arp)
	if err != nil {
		return nil
	}
	return &pdu.WireFrame{
		Frame: frame,
		Physical: &pdu.L1Metadata{
			SourceNodeID: r.ID,
			SourcePortID: portID,
		},
	}
}

// HandleIncomingArp parses incoming ARP packets using simulation time.
func (r *Router) HandleIncomingArp(incomingPort string, arp *pdu.ARPPacket, simTime time.Duration) (*pdu.WireFrame, []*pdu.IPv4Packet) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ArpCache[arp.SenderIP] = ArpEntry{
		MAC:       arp.SenderMAC,
		ExpiresAt: simTime + 20*time.Minute,
	}

	if arp.Operation == pdu.ArpRequest {
		ifaceIP, ok := r.Interfaces[incomingPort]
		if ok && ifaceIP == arp.TargetIP {
			replyArp := &pdu.ARPPacket{
				HardwareType: 1,
				ProtocolType: 0x0800,
				Operation:    pdu.ArpReply,
				SenderMAC:    r.InterfaceMAC[incomingPort],
				SenderIP:     ifaceIP,
				TargetMAC:    arp.SenderMAC,
				TargetIP:     arp.SenderIP,
			}
			frame, err := pdu.NewARPFrame(arp.SenderMAC, r.InterfaceMAC[incomingPort], replyArp)
			if err != nil {
				return nil, nil
			}
			wire := &pdu.WireFrame{
				ID:    "arp_reply_" + string(arp.SenderIP),
				Frame: frame,
				Physical: &pdu.L1Metadata{
					SourceNodeID: r.ID,
					SourcePortID: incomingPort,
				},
			}
			return wire, nil
		}
	}

	if arp.Operation == pdu.ArpReply {
		for _, ifaceIP := range r.Interfaces {
			if ifaceIP == arp.TargetIP {
				waiting := r.PacketQueue[arp.SenderIP]
				delete(r.PacketQueue, arp.SenderIP)
				return nil, waiting
			}
		}
	}

	return nil, nil
}
