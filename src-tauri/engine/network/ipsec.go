package network

import (
	"fmt"
	"strings"
	"time"

	"netforge/engine/pdu"
)

// ISAKMPPolicy is Phase 1 IKE policy.
type ISAKMPPolicy struct {
	Priority       int
	Encryption     string
	Hash           string
	Authentication string
	Group          int
	Lifetime       int
}

// IPSecTransformSet is Phase 2 transform-set.
type IPSecTransformSet struct {
	Name       string
	ESPAuth    string
	ESPEncrypt string
	Mode       string
}

// CryptoMapEntry binds IPsec to an interface ACL.
type CryptoMapEntry struct {
	Seq          int
	MapName      string
	PeerIP       pdu.IPAddress
	TransformSet string
	ACLName      string
	LocalSubnet  string
	RemoteSubnet string
	PFS          bool
}

// IKEPeerState tracks an established VPN peer.
type IKEPeerState struct {
	PeerIP       pdu.IPAddress
	State        pdu.IKEPhase1State
	TransformSet string
	LocalSubnet  string
	RemoteSubnet string
	SPIIn        uint32
	SPIOut       uint32
	Established  time.Duration
}

func (r *Router) AddISAKMPPolicy(p ISAKMPPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ISAKMPPolicies == nil {
		r.ISAKMPPolicies = make(map[int]*ISAKMPPolicy)
	}
	cp := p
	r.ISAKMPPolicies[p.Priority] = &cp
}

func (r *Router) SetISAKMPKey(peer pdu.IPAddress, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.PreSharedKeys == nil {
		r.PreSharedKeys = make(map[string]string)
	}
	r.PreSharedKeys[string(peer)] = key
}

func (r *Router) AddTransformSet(ts IPSecTransformSet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.TransformSets == nil {
		r.TransformSets = make(map[string]*IPSecTransformSet)
	}
	cp := ts
	r.TransformSets[ts.Name] = &cp
}

func (r *Router) AddCryptoMapEntry(e CryptoMapEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.CryptoMaps == nil {
		r.CryptoMaps = make(map[string][]CryptoMapEntry)
	}
	r.CryptoMaps[e.MapName] = append(r.CryptoMaps[e.MapName], e)
}

func (r *Router) ApplyCryptoMap(portID, mapName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IfaceCryptoMap == nil {
		r.IfaceCryptoMap = make(map[string]string)
	}
	r.IfaceCryptoMap[portID] = mapName
}

func (r *Router) MatchCryptoTunnel(portID string, ip *pdu.IPv4Packet) (*IKEPeerState, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	mapName := r.IfaceCryptoMap[portID]
	if mapName == "" {
		return nil, false
	}
	for _, e := range r.CryptoMaps[mapName] {
		if e.ACLName != "" && !r.evaluateACLLocked(e.ACLName, ip) {
			continue
		}
		peer := r.IKEPeers[string(e.PeerIP)]
		if peer != nil && peer.State == pdu.IKEActive {
			return peer, true
		}
	}
	return nil, false
}

func (r *Router) evaluateACLLocked(aclName string, ip *pdu.IPv4Packet) bool {
	acl, ok := r.ACLs[aclName]
	if !ok || len(acl.Rules) == 0 {
		return true
	}
	proto := "ip"
	if ip.Protocol == pdu.ProtoICMP {
		proto = "icmp"
	} else if ip.Protocol == pdu.ProtoUDP {
		proto = "udp"
	} else if ip.Protocol == pdu.ProtoTCP {
		proto = "tcp"
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

func (r *Router) EncapsulateESP(inner *pdu.IPv4Packet, peer *IKEPeerState, outPort string) *pdu.IPv4Packet {
	r.mu.RLock()
	peerIP := peer.PeerIP
	spi := peer.SPIOut
	srcIP := r.Interfaces[outPort]
	r.mu.RUnlock()
	return &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoESP,
		SourceIP: srcIP, DestinationIP: peerIP,
		ESP: &pdu.ESPPacket{
			SPI: spi, SeqNum: 1, PeerIP: peerIP,
			Transform: peer.TransformSet, Inner: inner,
		},
	}
}

func (r *Router) DecapsulateESP(outer *pdu.IPv4Packet) *pdu.IPv4Packet {
	if outer.ESP == nil || outer.ESP.Inner == nil {
		return nil
	}
	r.mu.RLock()
	peer := r.IKEPeers[string(outer.SourceIP)]
	r.mu.RUnlock()
	if peer == nil || peer.State != pdu.IKEActive {
		return nil
	}
	return outer.ESP.Inner
}

func (r *Router) FormatCryptoSA() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lines := []string{"Crypto ISAKMP SA", "dst             src             state"}
	for ip, peer := range r.IKEPeers {
		lines = append(lines, fmt.Sprintf("%-15s %-15s %s (spi %d/%d)",
			ip, string(r.publicIPLocked()), peer.State, peer.SPIIn, peer.SPIOut))
	}
	if len(r.IKEPeers) == 0 {
		lines = append(lines, "(no IKE SAs)")
	}
	return lines
}

func (r *Router) publicIPLocked() pdu.IPAddress {
	for port, ip := range r.Interfaces {
		if r.IfaceCryptoMap != nil && r.IfaceCryptoMap[port] != "" {
			return ip
		}
	}
	for _, ip := range r.Interfaces {
		if !isPrivateIP(ip) {
			return ip
		}
	}
	for _, ip := range r.Interfaces {
		return ip
	}
	return ""
}

func isPrivateIP(ip pdu.IPAddress) bool {
	s := string(ip)
	return strings.HasPrefix(s, "10.") || strings.HasPrefix(s, "192.168.") || strings.HasPrefix(s, "172.")
}

func (r *Router) publicIP() pdu.IPAddress {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.publicIPLocked()
}

func (r *Router) getPSK(peer pdu.IPAddress) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.PreSharedKeys == nil {
		return ""
	}
	return r.PreSharedKeys[string(peer)]
}

func (r *Router) activatePeer(peerIP pdu.IPAddress, simTime time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IKEPeers == nil {
		r.IKEPeers = make(map[string]*IKEPeerState)
	}
	ts := "TS"
	for name := range r.TransformSets {
		ts = name
		break
	}
	localSub, remoteSub := "", ""
	for _, entries := range r.CryptoMaps {
		for _, e := range entries {
			if e.PeerIP == peerIP {
				localSub = e.LocalSubnet
				remoteSub = e.RemoteSubnet
				if e.TransformSet != "" {
					ts = e.TransformSet
				}
			}
		}
	}
	r.IKEPeers[string(peerIP)] = &IKEPeerState{
		PeerIP: peerIP, State: pdu.IKEActive, TransformSet: ts,
		LocalSubnet: localSub, RemoteSubnet: remoteSub,
		SPIIn: 0x1001, SPIOut: 0x2001, Established: simTime,
	}
}

func (m *Manager) NegotiateIKE(localRouterID string, peerIP pdu.IPAddress, psk string) error {
	local, ok := m.GetRouter(localRouterID)
	if !ok {
		return errDeviceNotFound(localRouterID)
	}
	remote := m.findRouterByPeerIP(peerIP)
	if remote == nil {
		return fmt.Errorf("peer router for %s not found", peerIP)
	}
	localKey := local.getPSK(peerIP)
	remoteKey := remote.getPSK(local.publicIP())
	if psk != "" {
		localKey = psk
	}
	if localKey == "" || remoteKey == "" || localKey != remoteKey {
		return fmt.Errorf("IKE authentication failed: PSK mismatch")
	}
	simTime := m.SimNow()
	local.activatePeer(peerIP, simTime)
	remote.activatePeer(local.publicIP(), simTime)
	m.LogEvent(EventProtocol, localRouterID, "", fmt.Sprintf("IKE SA established with %s", peerIP), nil)
	return nil
}

func (m *Manager) findRouterByPeerIP(ip pdu.IPAddress) *Router {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.Routers {
		if r.OwnsIP(ip) {
			return r
		}
	}
	return nil
}

func (m *Manager) InstallVPNRoutes(routerID string) {
	r, ok := m.GetRouter(routerID)
	if !ok {
		return
	}
	r.mu.RLock()
	peers := make([]*IKEPeerState, 0, len(r.IKEPeers))
	for _, p := range r.IKEPeers {
		peers = append(peers, p)
	}
	r.mu.RUnlock()
	for _, peer := range peers {
		if peer.State == pdu.IKEActive && peer.RemoteSubnet != "" {
			_ = m.AddStaticRoute(routerID, peer.RemoteSubnet, string(peer.PeerIP), "", 1)
		}
	}
}

func (r *Router) HasActiveVPN() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.IKEPeers {
		if p.State == pdu.IKEActive {
			return true
		}
	}
	return false
}
