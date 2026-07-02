package network

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"netforge/engine/pdu"
)

// ACLAction is permit or deny.
type ACLAction string

const (
	ACLPermit ACLAction = "permit"
	ACLDeny   ACLAction = "deny"
)

// ACLRule is a single access-list entry.
type ACLRule struct {
	Action   ACLAction
	Protocol string
	SrcNet   string
	DstNet   string
	SrcPort  int
	DstPort  int
}

// ACL is a numbered or named access control list.
type ACL struct {
	Number int
	Name   string
	Rules  []ACLRule
}

// IfacePolicy holds per-interface ACL and NAT roles.
type IfacePolicy struct {
	Up          bool
	Shutdown    bool
	InboundACL  string
	OutboundACL string
	NATInside   bool
	NATOutside  bool
	VLANID      pdu.VLANID
}

// NATEntry is a translation mapping.
type NATEntry struct {
	InsideLocal   pdu.IPAddress
	InsideGlobal  pdu.IPAddress
	OutsideLocal  pdu.IPAddress
	OutsideGlobal pdu.IPAddress
	Protocol      string
	InsidePort    int
	OutsidePort   int
	Static        bool
}

// NATTable manages static, dynamic, and PAT translations.
type NATTable struct {
	Overload    bool
	Static      []NATEntry
	Dynamic     []NATEntry
	PAT         map[string]NATEntry
	NextPATPort int
	mu          sync.RWMutex
}

// DHCPPool defines an address pool on a router.
type DHCPPool struct {
	Name          string
	Network       string
	DefaultRouter pdu.IPAddress
	DNSServer     pdu.IPAddress
	VoIPOption150 pdu.IPAddress
	VoiceVLAN     pdu.VLANID
	LeaseTime     int
	Assigned      map[string]pdu.IPAddress
	Excluded      []string
}

// DNSServerDB holds static DNS records.
type DNSServerDB struct {
	Records map[string]pdu.IPAddress
}

// CDPNeighbor is a discovered CDP neighbor.
type CDPNeighbor struct {
	DeviceID    string
	PortID      string
	Platform    string
	IPAddress   pdu.IPAddress
	LocalPort   string
	LastSeen    int64
}

// NewNATTable creates an empty NAT table.
func NewNATTable() *NATTable {
	return &NATTable{PAT: make(map[string]NATEntry), NextPATPort: 1024}
}

func (r *Router) AddACLRule(aclName string, rule ACLRule) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ACLs == nil {
		r.ACLs = make(map[string]*ACL)
	}
	acl, ok := r.ACLs[aclName]
	if !ok {
		acl = &ACL{Name: aclName, Rules: make([]ACLRule, 0)}
		r.ACLs[aclName] = acl
	}
	acl.Rules = append(acl.Rules, rule)
}

func (r *Router) SetIfacePolicy(portID string, policy IfacePolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IfacePolicies == nil {
		r.IfacePolicies = make(map[string]*IfacePolicy)
	}
	p := policy
	r.IfacePolicies[portID] = &p
}

func (r *Router) SetInterfaceShutdown(portID string, shutdown bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IfacePolicies == nil {
		r.IfacePolicies = make(map[string]*IfacePolicy)
	}
	pol, ok := r.IfacePolicies[portID]
	if !ok {
		pol = &IfacePolicy{Up: true}
		r.IfacePolicies[portID] = pol
	}
	pol.Shutdown = shutdown
	pol.Up = !shutdown
}

func (r *Router) IsInterfaceUp(portID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if pol, ok := r.IfacePolicies[portID]; ok {
		return pol.Up && !pol.Shutdown
	}
	return true
}

func (r *Router) GetInboundACL(portID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if pol, ok := r.IfacePolicies[portID]; ok {
		return pol.InboundACL
	}
	return ""
}

func (r *Router) GetOutboundACL(portID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if pol, ok := r.IfacePolicies[portID]; ok {
		return pol.OutboundACL
	}
	return ""
}

func (r *Router) ApplyIfaceACL(portID, direction, aclName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IfacePolicies == nil {
		r.IfacePolicies = make(map[string]*IfacePolicy)
	}
	pol, ok := r.IfacePolicies[portID]
	if !ok {
		pol = &IfacePolicy{Up: true}
		r.IfacePolicies[portID] = pol
	}
	if direction == "in" {
		pol.InboundACL = aclName
	} else {
		pol.OutboundACL = aclName
	}
}

func (r *Router) EvaluateACL(aclName string, ip *pdu.IPv4Packet) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if aclName == "" {
		return true
	}
	acl, ok := r.ACLs[aclName]
	if !ok || len(acl.Rules) == 0 {
		return true
	}

	proto := "ip"
	if ip.Protocol == pdu.ProtoICMP {
		proto = "icmp"
	}

	for _, rule := range acl.Rules {
		if rule.Protocol != "ip" && rule.Protocol != proto {
			continue
		}
		if !matchACLNet(rule.SrcNet, ip.SourceIP) {
			continue
		}
		if !matchACLNet(rule.DstNet, ip.DestinationIP) {
			continue
		}
		return rule.Action == ACLPermit
	}
	return false
}

func matchACLNet(cidr string, ip pdu.IPAddress) bool {
	if cidr == "" || cidr == "any" {
		return true
	}
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return string(ip) == cidr
	}
	return ipNet.Contains(net.ParseIP(string(ip)))
}

func (r *Router) TranslateOutbound(portID string, ip *pdu.IPv4Packet) *pdu.IPv4Packet {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.NAT == nil {
		return ip
	}
	pol := r.IfacePolicies[portID]
	if pol == nil || !pol.NATOutside {
		return ip
	}

	translated := *ip
	src := ip.SourceIP

	for _, entry := range r.NAT.Static {
		if entry.InsideLocal == src {
			translated.SourceIP = entry.InsideGlobal
			return &translated
		}
	}

	if r.NAT.Overload {
		key := string(src)
		if entry, ok := r.NAT.PAT[key]; ok {
			translated.SourceIP = entry.OutsideGlobal
			return &translated
		}
		globalIP := r.pickOutsideIPLocked()
		entry := NATEntry{InsideLocal: src, InsideGlobal: globalIP, OutsideGlobal: globalIP}
		r.NAT.PAT[key] = entry
		translated.SourceIP = globalIP
		return &translated
	}
	return ip
}

func (r *Router) TranslateInbound(portID string, ip *pdu.IPv4Packet) *pdu.IPv4Packet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.NAT == nil {
		return ip
	}
	pol := r.IfacePolicies[portID]
	if pol == nil || !pol.NATOutside {
		return ip
	}

	translated := *ip
	dst := ip.DestinationIP

	for _, entry := range r.NAT.Static {
		if entry.InsideGlobal == dst || entry.OutsideGlobal == dst {
			translated.DestinationIP = entry.InsideLocal
			return &translated
		}
	}
	for _, entry := range r.NAT.PAT {
		if entry.OutsideGlobal == dst {
			translated.DestinationIP = entry.InsideLocal
			return &translated
		}
	}
	return ip
}

func (r *Router) pickOutsideIPLocked() pdu.IPAddress {
	for portID, ip := range r.Interfaces {
		if pol, ok := r.IfacePolicies[portID]; ok && pol.NATOutside {
			return ip
		}
	}
	for _, ip := range r.Interfaces {
		return ip
	}
	return "0.0.0.0"
}

func (r *Router) AddStaticNAT(insideLocal, insideGlobal pdu.IPAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.NAT == nil {
		r.NAT = NewNATTable()
	}
	r.NAT.Static = append(r.NAT.Static, NATEntry{
		InsideLocal: insideLocal, InsideGlobal: insideGlobal,
		OutsideGlobal: insideGlobal, Static: true,
	})
}

func (r *Router) EnableNATOverload(outsidePort string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.NAT == nil {
		r.NAT = NewNATTable()
	}
	r.NAT.Overload = true
	if r.IfacePolicies == nil {
		r.IfacePolicies = make(map[string]*IfacePolicy)
	}
	pol := r.IfacePolicies[outsidePort]
	if pol == nil {
		pol = &IfacePolicy{Up: true}
		r.IfacePolicies[outsidePort] = pol
	}
	pol.NATOutside = true
}

func (r *Router) MarkNATOutside(portID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IfacePolicies == nil {
		r.IfacePolicies = make(map[string]*IfacePolicy)
	}
	pol := r.IfacePolicies[portID]
	if pol == nil {
		pol = &IfacePolicy{Up: true}
		r.IfacePolicies[portID] = pol
	}
	pol.NATOutside = true
}

func (r *Router) MarkNATInside(portID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IfacePolicies == nil {
		r.IfacePolicies = make(map[string]*IfacePolicy)
	}
	pol := r.IfacePolicies[portID]
	if pol == nil {
		pol = &IfacePolicy{Up: true}
		r.IfacePolicies[portID] = pol
	}
	pol.NATInside = true
}

func (r *Router) SetDHCPPoolDefaultRouter(poolName string, gw pdu.IPAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.DHCPPools == nil {
		r.DHCPPools = make(map[string]*DHCPPool)
	}
	if r.DHCPPools[poolName] == nil {
		r.DHCPPools[poolName] = &DHCPPool{Name: poolName, Assigned: make(map[string]pdu.IPAddress)}
	}
	r.DHCPPools[poolName].DefaultRouter = gw
}

func (r *Router) SetDHCPPoolNetwork(poolName, cidr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.DHCPPools == nil {
		r.DHCPPools = make(map[string]*DHCPPool)
	}
	if r.DHCPPools[poolName] == nil {
		r.DHCPPools[poolName] = &DHCPPool{Name: poolName, Assigned: make(map[string]pdu.IPAddress)}
	}
	r.DHCPPools[poolName].Network = cidr
}

func (r *Router) AddDHCPPool(pool DHCPPool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.DHCPPools == nil {
		r.DHCPPools = make(map[string]*DHCPPool)
	}
	if pool.Assigned == nil {
		pool.Assigned = make(map[string]pdu.IPAddress)
	}
	p := pool
	r.DHCPPools[pool.Name] = &p
}

func (r *Router) HandleDHCPDiscover(dhcp *pdu.DHCPPacket, inPort string) *pdu.DHCPPacket {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, pool := range r.DHCPPools {
		_, ipNet, err := net.ParseCIDR(pool.Network)
		if err != nil {
			continue
		}
		ifaceIP, ok := r.Interfaces[inPort]
		if !ok || !ipNet.Contains(net.ParseIP(string(ifaceIP))) {
			continue
		}

		assignedIP := pool.Assigned[string(dhcp.ClientMAC)]
		if assignedIP == "" {
			assignedIP = r.nextPoolIPLocked(pool, ipNet)
			pool.Assigned[string(dhcp.ClientMAC)] = assignedIP
		}

		return &pdu.DHCPPacket{
			Op: 2, MessageType: pdu.DHCPOffer, XID: dhcp.XID,
			YourIP: assignedIP, ServerIP: ifaceIP,
			GatewayIP: pool.DefaultRouter, DNSServer: pool.DNSServer,
			SubnetMask: maskToDotted(ipNet.Mask),
		}
	}
	return nil
}

func (r *Router) HandleDHCPRequest(dhcp *pdu.DHCPPacket, inPort string) *pdu.DHCPPacket {
	offer := r.HandleDHCPDiscover(dhcp, inPort)
	if offer == nil {
		return nil
	}
	ack := *offer
	ack.MessageType = pdu.DHCPAck
	return &ack
}

func (r *Router) nextPoolIPLocked(pool *DHCPPool, ipNet *net.IPNet) pdu.IPAddress {
	ip := ipNet.IP.To4()
	for host := 2; host < 254; host++ {
		candidate := net.IPv4(ip[0], ip[1], ip[2], byte(host))
		if !ipNet.Contains(candidate) {
			continue
		}
		inUse := false
		for _, assigned := range pool.Assigned {
			if assigned == pdu.IPAddress(candidate.String()) {
				inUse = true
				break
			}
		}
		for _, ifaceIP := range r.Interfaces {
			if ifaceIP == pdu.IPAddress(candidate.String()) {
				inUse = true
				break
			}
		}
		if !inUse && r.isIPExcludedLocked(candidate.String(), pool) {
			inUse = true
		}
		if !inUse {
			return pdu.IPAddress(candidate.String())
		}
	}
	return pdu.IPAddress(ip.String())
}

func (r *Router) isIPExcludedLocked(ip string, pool *DHCPPool) bool {
	ranges := append([]string{}, pool.Excluded...)
	ranges = append(ranges, r.DHCPExcluded...)
	for _, excl := range ranges {
		parts := strings.Fields(excl)
		if len(parts) == 1 {
			if parts[0] == ip {
				return true
			}
		}
		if len(parts) >= 2 {
			if ipInRange(ip, parts[0], parts[1]) {
				return true
			}
		}
	}
	return false
}

func ipInRange(ip, start, end string) bool {
	a := net.ParseIP(ip)
	s := net.ParseIP(start)
	e := net.ParseIP(end)
	if a == nil || s == nil || e == nil {
		return false
	}
	ai := ipToUint32(a)
	si := ipToUint32(s)
	ei := ipToUint32(e)
	return ai >= si && ai <= ei
}

func ipToUint32(ip net.IP) uint32 {
	v4 := ip.To4()
	if v4 == nil {
		return 0
	}
	return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
}

func maskToDotted(mask net.IPMask) string {
	if len(mask) == 4 {
		return net.IP(mask).String()
	}
	return "255.255.255.0"
}

func (r *Router) LookupDNS(name string) (pdu.IPAddress, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.DNS == nil {
		return "", false
	}
	ip, ok := r.DNS.Records[name]
	return ip, ok
}

func (r *Router) AddDNSRecord(name string, ip pdu.IPAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.DNS == nil {
		r.DNS = &DNSServerDB{Records: make(map[string]pdu.IPAddress)}
	}
	r.DNS.Records[name] = ip
}

func (r *Router) AddSubInterface(parentPort string, vlan int, ip pdu.IPAddress, mask string) string {
	subID := fmt.Sprintf("%s.%d", parentPort, vlan)
	mac := pdu.MACAddress(fmt.Sprintf("00:1A:2B:00:%02X:01", vlan))
	r.AddInterface(subID, ip, mask, mac)
	r.SetIfacePolicy(subID, IfacePolicy{Up: true, VLANID: pdu.VLANID(vlan)})
	return subID
}

func (r *Router) BuildCDPAdvertisement(portID string) *pdu.CDPPacket {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return &pdu.CDPPacket{
		DeviceID: r.ID, PortID: portID, Platform: "NetForge-ISR",
		IPAddress: r.Interfaces[portID], Capabilities: "Router",
	}
}

func (r *Router) LearnCDPNeighbor(localPort string, pkt *pdu.CDPPacket) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CDPNeighbors == nil {
		r.CDPNeighbors = make([]CDPNeighbor, 0)
	}
	r.CDPNeighbors = append(r.CDPNeighbors, CDPNeighbor{
		DeviceID: pkt.DeviceID, PortID: pkt.PortID, Platform: pkt.Platform,
		IPAddress: pkt.IPAddress, LocalPort: localPort,
	})
}
