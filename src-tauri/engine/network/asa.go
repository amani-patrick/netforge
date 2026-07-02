package network

import (
	"net"
	"sync"
	"time"

	"netforge/engine/pdu"
)

// SecurityZone is inside, outside, or DMZ.
type SecurityZone string

const (
	ZoneInside  SecurityZone = "inside"
	ZoneOutside SecurityZone = "outside"
	ZoneDMZ     SecurityZone = "dmz"
)

// ASAConnection tracks stateful flow.
type ASAConnection struct {
	SrcIP      pdu.IPAddress
	DstIP      pdu.IPAddress
	SrcPort    int
	DstPort    int
	Protocol   uint8
	ExpiresAt  time.Duration
}

// ASAFirewall is a simplified Cisco ASA appliance.
type ASAFirewall struct {
	ID            string
	Interfaces    map[string]pdu.IPAddress
	InterfaceMAC  map[string]pdu.MACAddress
	InterfaceMask map[string]string
	InterfaceZone map[string]SecurityZone
	Rules         []ACLRule
	ConnTable     map[string]ASAConnection
	InspectionOn  bool
	mu            sync.RWMutex
}

// NewASAFirewall creates an ASA device.
func NewASAFirewall(id string) *ASAFirewall {
	return &ASAFirewall{
		ID: id, Interfaces: make(map[string]pdu.IPAddress),
		InterfaceMAC: make(map[string]pdu.MACAddress),
		InterfaceMask: make(map[string]string),
		InterfaceZone: make(map[string]SecurityZone),
		ConnTable: make(map[string]ASAConnection),
		InspectionOn: true,
	}
}

// AddInterface configures an ASA interface with zone.
func (a *ASAFirewall) AddInterface(portID string, ip pdu.IPAddress, mask string, mac pdu.MACAddress, zone SecurityZone) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Interfaces[portID] = ip
	a.InterfaceMask[portID] = mask
	a.InterfaceMAC[portID] = mac
	a.InterfaceZone[portID] = zone
}

// AddRule adds an ACL rule to the ASA.
func (a *ASAFirewall) AddRule(rule ACLRule) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Rules = append(a.Rules, rule)
}

// InspectPacket applies stateful firewall policy.
func (a *ASAFirewall) InspectPacket(inPort string, ip *pdu.IPv4Packet, simTime time.Duration) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	zone := a.InterfaceZone[inPort]
	key := connKey(ip.SourceIP, ip.DestinationIP, ip.Protocol)

	if entry, ok := a.ConnTable[key]; ok && simTime < entry.ExpiresAt {
		return true
	}

	permitted := a.evaluateRules(ip, zone)
	if permitted && a.InspectionOn {
		a.ConnTable[key] = ASAConnection{
			SrcIP: ip.SourceIP, DstIP: ip.DestinationIP, Protocol: ip.Protocol,
			ExpiresAt: simTime + 30*time.Minute,
		}
	}
	return permitted
}

func (a *ASAFirewall) evaluateRules(ip *pdu.IPv4Packet, zone SecurityZone) bool {
	if len(a.Rules) == 0 {
		return zone != ZoneOutside // default deny inbound from outside
	}
	proto := "ip"
	if ip.Protocol == pdu.ProtoICMP {
		proto = "icmp"
	}
	for _, rule := range a.Rules {
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

func connKey(src, dst pdu.IPAddress, proto uint8) string {
	return string(src) + "->" + string(dst) + "/" + string(rune(proto))
}

// OwnsIP checks if packet is destined to ASA interface.
func (a *ASAFirewall) OwnsIP(ip pdu.IPAddress) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, ifaceIP := range a.Interfaces {
		if ifaceIP == ip {
			return true
		}
	}
	return false
}

// MatchRoute simple LPM for ASA forwarding.
func (a *ASAFirewall) MatchRoute(dest pdu.IPAddress) (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	target := net.ParseIP(string(dest))
	bestPort := ""
	bestLen := -1
	for portID, ip := range a.Interfaces {
		mask := a.InterfaceMask[portID]
		maskIP := net.IPMask(net.ParseIP(mask).To4())
		netIP := &net.IPNet{IP: net.ParseIP(string(ip)).Mask(maskIP), Mask: maskIP}
		if netIP.Contains(target) {
			ones, _ := netIP.Mask.Size()
			if ones > bestLen {
				bestLen = ones
				bestPort = portID
			}
		}
	}
	return bestPort, bestPort != ""
}
