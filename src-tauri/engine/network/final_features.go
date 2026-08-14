package network

import (
	"fmt"
	"time"

	"netforge/engine"
	"netforge/engine/pdu"
)

// AddAccessPoint provisions a wireless AP.
func (m *Manager) AddAccessPoint(id string) *AccessPoint {
	m.mu.Lock()
	defer m.mu.Unlock()
	ap := NewAccessPoint(id)
	m.AccessPoints[id] = ap
	return ap
}

// GetAccessPoint returns an AP by ID.
func (m *Manager) GetAccessPoint(id string) (*AccessPoint, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ap, ok := m.AccessPoints[id]
	return ap, ok
}

// AddASAFirewall provisions an ASA appliance.
func (m *Manager) AddASAFirewall(id string) *ASAFirewall {
	m.mu.Lock()
	defer m.mu.Unlock()
	asa := NewASAFirewall(id)
	m.ASAFirewalls[id] = asa
	return asa
}

// GetASAFirewall returns an ASA by ID.
func (m *Manager) GetASAFirewall(id string) (*ASAFirewall, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	asa, ok := m.ASAFirewalls[id]
	return asa, ok
}

// ConfigureWANSerial sets encapsulation on a serial interface.
// When both ends of an existing link are now configured as PPP, the handshake
// is kicked off automatically so the link comes up without manual intervention.
func (m *Manager) ConfigureWANSerial(routerID, portID, encap string, bandwidth int64) {
	m.WAN.ConfigureSerial(routerID, portID, encap, bandwidth)
	if encap == "ppp" {
		// Check whether the peer port is also configured as PPP — if so, start LCP.
		destNode, destPort, _, ok := m.ResolveLinkPeer(routerID, portID)
		if ok {
			peerSerial := m.WAN.GetSerial(destNode, destPort)
			if peerSerial != nil && peerSerial.Encap == "ppp" && peerSerial.PPPState == PPPDead {
				go m.InitiatePPPHandshake(routerID, portID)
			}
		}
	}
}

// ConfigureFrameRelayMap registers a Frame Relay PVC.
func (m *Manager) ConfigureFrameRelayMap(link WANLink) {
	m.WAN.AddFRLink(link)
}

// WirelessAssociate authenticates a client to an AP.
func (m *Manager) WirelessAssociate(apID string, clientMAC pdu.MACAddress, password string) error {
	ap, ok := m.GetAccessPoint(apID)
	if !ok {
		return errDeviceNotFound(apID)
	}
	if !ap.Associate(clientMAC, password) {
		return fmt.Errorf("wireless association failed")
	}
	return nil
}

// StartPing6 initiates ICMPv6 echo from a host.
func (m *Manager) StartPing6(sourceID, destIP, requestID string) error {
	host, ok := m.GetHost(sourceID)
	if !ok {
		router, rOk := m.GetRouter(sourceID)
		if !rOk {
			return fmt.Errorf("device %s not found", sourceID)
		}
		return m.startRouterPing6(router, destIP, requestID)
	}

	simTime := m.SimNow()
	sessionID := fmt.Sprintf("ping6_%s_%d", sourceID, simTime)
	icmpID := uint16(simTime/time.Millisecond) % 65535
	seq := uint16(1)

	m.mu.Lock()
	m.pingSessions[sessionID] = &PingSession{
		ID: sessionID, SourceID: sourceID, DestIP: destIP,
		ICMPID: icmpID, Sequence: seq, SentAt: simTime,
		ReplyConn: ReplyTarget{RequestID: requestID},
	}
	m.mu.Unlock()

	echo := pdu.NewEchoRequest(icmpID, seq, []byte("NetForge ping6"))
	ipPkt := &pdu.IPv6Packet{
		HopLimit: 64, NextHeader: pdu.ProtoICMPv6,
		SourceIP: host.IPv6, DestinationIP: pdu.IPv6Address(destIP),
		ICMPv6: echo,
	}
	m.sendIPv6FromHost(host, ipPkt, simTime)

	if m.scheduler != nil {
		m.scheduler.Schedule(engine.EventTimerICMP, engine.IcmpPingTimeout, sessionID)
	}
	return nil
}

func (m *Manager) startRouterPing6(router *Router, destIP, requestID string) error {
	portID := ""
	for p := range router.IPv6Interfaces {
		portID = p
		break
	}
	if portID == "" {
		return fmt.Errorf("no IPv6 interface configured")
	}
	simTime := m.SimNow()
	echo := pdu.NewEchoRequest(1, 1, []byte("ping6"))
	ipPkt := &pdu.IPv6Packet{
		HopLimit: 64, NextHeader: pdu.ProtoICMPv6,
		SourceIP: router.IPv6Interfaces[portID], DestinationIP: pdu.IPv6Address(destIP),
		ICMPv6: echo,
	}
	m.forwardIPv6FromRouter(router, ipPkt, simTime)
	_ = requestID
	return nil
}
