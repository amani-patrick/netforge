package network

import (
	"net"
	"sync"
	"time"

	"netforge/engine/pdu"
)

// HostArpEntry maps an IP to a MAC with sim-time expiry.
type HostArpEntry struct {
	MAC       pdu.MACAddress
	ExpiresAt time.Duration
}

// Host represents an end-station (PC) with a minimal IP stack.
type Host struct {
	ID          string
	IP          pdu.IPAddress
	IPv6        pdu.IPv6Address
	Mask        string
	Gateway     pdu.IPAddress
	IPv6Gateway pdu.IPv6Address
	MAC         pdu.MACAddress
	PortID      string
	UplinkNode  string
	ArpCache    map[pdu.IPAddress]HostArpEntry
	NDPCache    map[pdu.IPv6Address]HostArpEntry
	PacketQueue map[pdu.IPAddress][]*pdu.IPv4Packet
	IPv6Queue   map[pdu.IPv6Address][]*pdu.IPv6Packet
	mu          sync.RWMutex
}

// NewHost creates a new end host.
func NewHost(id string) *Host {
	return &Host{
		ID:          id,
		PortID:      "FastEthernet0",
		ArpCache:    make(map[pdu.IPAddress]HostArpEntry),
		NDPCache:    make(map[pdu.IPv6Address]HostArpEntry),
		PacketQueue: make(map[pdu.IPAddress][]*pdu.IPv4Packet),
		IPv6Queue:   make(map[pdu.IPv6Address][]*pdu.IPv6Packet),
	}
}

// ConfigureIPv6 sets host IPv6 parameters.
func (h *Host) ConfigureIPv6(ip pdu.IPv6Address, gateway pdu.IPv6Address) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.IPv6 = ip
	h.IPv6Gateway = gateway
}

// OwnsIPv6 returns true if the given IPv6 is assigned to this host.
func (h *Host) OwnsIPv6(ip pdu.IPv6Address) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.IPv6 == ip
}

// LookupNDP returns cached neighbor MAC for IPv6.
func (h *Host) LookupNDP(ip pdu.IPv6Address, simTime time.Duration) (pdu.MACAddress, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	entry, ok := h.NDPCache[ip]
	if !ok || simTime > entry.ExpiresAt {
		return "", false
	}
	return entry.MAC, true
}

// LearnNDP records IPv6 neighbor binding.
func (h *Host) LearnNDP(ip pdu.IPv6Address, mac pdu.MACAddress, simTime time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.NDPCache[ip] = HostArpEntry{MAC: mac, ExpiresAt: simTime + 20*time.Minute}
}

// QueueIPv6Packet buffers IPv6 pending NDP resolution.
func (h *Host) QueueIPv6Packet(nextHop pdu.IPv6Address, pkt *pdu.IPv6Packet) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.IPv6Queue[nextHop] = append(h.IPv6Queue[nextHop], pkt)
}

// DequeueIPv6 returns queued IPv6 packets for a neighbor.
func (h *Host) DequeueIPv6(nextHop pdu.IPv6Address) []*pdu.IPv6Packet {
	h.mu.Lock()
	defer h.mu.Unlock()
	pkts := h.IPv6Queue[nextHop]
	delete(h.IPv6Queue, nextHop)
	return pkts
}

// ResolveNextHopIPv6 returns L3 next-hop for IPv6 destination.
func (h *Host) ResolveNextHopIPv6(dest pdu.IPv6Address) pdu.IPv6Address {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.IPv6Gateway != "" {
		return h.IPv6Gateway
	}
	return dest
}

// Configure sets host L3 parameters.
func (h *Host) Configure(ip pdu.IPAddress, mask string, gateway pdu.IPAddress, mac pdu.MACAddress) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.IP = ip
	h.Mask = mask
	h.Gateway = gateway
	h.MAC = mac
}

// SetUplink records which node this host is cabled to.
func (h *Host) SetUplink(nodeID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.UplinkNode = nodeID
}

// IsLocalAddress returns true if the IP is on the host's subnet.
func (h *Host) IsLocalAddress(ip pdu.IPAddress) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	ipAddr := net.ParseIP(string(ip))
	maskAddr := net.IPMask(net.ParseIP(h.Mask).To4())
	if ipAddr == nil || maskAddr == nil {
		return false
	}
	network := ipAddr.Mask(maskAddr)
	hostIP := net.ParseIP(string(h.IP))
	return network.Equal(hostIP.Mask(maskAddr))
}

// ResolveNextHopIP returns the L3 next-hop for a destination.
func (h *Host) ResolveNextHopIP(dest pdu.IPAddress) pdu.IPAddress {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.isLocalLocked(dest) {
		return dest
	}
	if h.Gateway != "" {
		return h.Gateway
	}
	return dest
}

func (h *Host) isLocalLocked(ip pdu.IPAddress) bool {
	ipAddr := net.ParseIP(string(ip))
	maskAddr := net.IPMask(net.ParseIP(h.Mask).To4())
	if ipAddr == nil || maskAddr == nil {
		return false
	}
	hostIP := net.ParseIP(string(h.IP))
	return ipAddr.Mask(maskAddr).Equal(hostIP.Mask(maskAddr))
}

// LookupARP returns a cached MAC if still valid at simTime.
func (h *Host) LookupARP(ip pdu.IPAddress, simTime time.Duration) (pdu.MACAddress, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entry, ok := h.ArpCache[ip]
	if !ok || simTime > entry.ExpiresAt {
		return "", false
	}
	return entry.MAC, true
}

// LearnARP records a MAC/IP binding at simTime.
func (h *Host) LearnARP(ip pdu.IPAddress, mac pdu.MACAddress, simTime time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ArpCache[ip] = HostArpEntry{
		MAC:       mac,
		ExpiresAt: simTime + 20*time.Minute,
	}
}

// QueuePacket buffers an IP packet waiting for ARP resolution.
func (h *Host) QueuePacket(nextHop pdu.IPAddress, pkt *pdu.IPv4Packet) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.PacketQueue[nextHop] = append(h.PacketQueue[nextHop], pkt)
}

// DequeuePackets returns and clears packets waiting for a resolved next-hop.
func (h *Host) DequeuePackets(nextHop pdu.IPAddress) []*pdu.IPv4Packet {
	h.mu.Lock()
	defer h.mu.Unlock()
	pkts := h.PacketQueue[nextHop]
	delete(h.PacketQueue, nextHop)
	return pkts
}

// OwnsIP returns true if the given IP is assigned to this host.
func (h *Host) OwnsIP(ip pdu.IPAddress) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.IP == ip
}

// Snapshot is a JSON-serializable host state.
type HostSnapshot struct {
	ID         string `json:"id"`
	IP         string `json:"ip"`
	Mask       string `json:"mask"`
	Gateway    string `json:"gateway"`
	MAC        string `json:"mac"`
	PortID     string `json:"port_id"`
	UplinkNode string `json:"uplink_node"`
}

// Snapshot exports host configuration.
func (h *Host) Snapshot() HostSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return HostSnapshot{
		ID:         h.ID,
		IP:         string(h.IP),
		Mask:       h.Mask,
		Gateway:    string(h.Gateway),
		MAC:        string(h.MAC),
		PortID:     h.PortID,
		UplinkNode: h.UplinkNode,
	}
}

// RestoreHost loads host state from a snapshot.
func RestoreHost(s HostSnapshot) *Host {
	h := NewHost(s.ID)
	h.Configure(pdu.IPAddress(s.IP), s.Mask, pdu.IPAddress(s.Gateway), pdu.MACAddress(s.MAC))
	h.PortID = s.PortID
	h.SetUplink(s.UplinkNode)
	return h
}
