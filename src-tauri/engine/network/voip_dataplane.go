package network

import (
	"fmt"
	"time"

	"netforge/engine"
	"netforge/engine/pdu"
)

// handleLocalIPv4 processes UDP VoIP signaling/media delivered to a local IP.
func (m *Manager) handleLocalIPv4(nodeID, portID string, router *Router, host *Host, ip *pdu.IPv4Packet, simTime time.Duration) bool {
	if ip.Protocol != pdu.ProtoUDP {
		return false
	}
	cmID, cm := m.findCallManagerByIP(ip.DestinationIP)
	if cm != nil {
		return m.handleVoIPAtCallManager(cmID, cm, nodeID, portID, router, host, ip, simTime)
	}
	return false
}

func (m *Manager) findCallManagerByIP(ip pdu.IPAddress) (string, *CallManager) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, cm := range m.CallManagers {
		if cm.IP == ip {
			return id, cm
		}
	}
	return "", nil
}

func (m *Manager) handleVoIPAtCallManager(cmID string, cm *CallManager, nodeID, portID string, router *Router, host *Host, ip *pdu.IPv4Packet, simTime time.Duration) bool {
	replyIP := (*pdu.IPv4Packet)(nil)

	if ip.SIP != nil {
		reply := m.HandleSIP(cmID, ip.SIP, simTime)
		if reply != nil {
			replyIP = m.buildSIPReplyIP(cm.IP, ip, reply)
			if reply.Method == pdu.SIPRinging {
				m.scheduleSIPAck(ip, simTime)
			}
		}
	} else if ip.SCCP != nil {
		phone := m.findPhoneByIP(ip.SourceIP)
		reply := m.HandleSCCP(cmID, ip.SCCP, phone, simTime)
		if reply != nil {
			replyIP = m.buildSCCPReplyIP(cm.IP, ip, reply)
		}
	} else if ip.RTP != nil {
		m.LogEvent(EventProtocol, cmID, "", fmt.Sprintf("RTP %s %d bytes", ip.RTP.Codec, ip.RTP.PayloadLen), nil)
		return true
	}

	if replyIP == nil {
		return ip.SIP != nil || ip.SCCP != nil || ip.RTP != nil
	}
	m.sendVoIPReply(nodeID, portID, router, host, replyIP, simTime)
	return true
}

func (m *Manager) scheduleSIPAck(inviteIP *pdu.IPv4Packet, simTime time.Duration) {
	if inviteIP.SIP == nil || m.scheduler == nil {
		return
	}
	callID := inviteIP.SIP.CallID
	ack := &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoUDP,
		SourceIP: inviteIP.SourceIP, DestinationIP: inviteIP.DestinationIP,
		SrcPort: inviteIP.DstPort, DstPort: inviteIP.SrcPort,
		DSCP: pdu.DSCPEF,
		SIP: &pdu.SIPPacket{Method: pdu.SIPAck, CallID: callID, From: inviteIP.SIP.From, To: inviteIP.SIP.To},
	}
	m.scheduler.Schedule(engine.EventTimerVoIP, 50*time.Millisecond, &sipAckJob{mgr: m, ack: ack, phoneIP: inviteIP.SourceIP})
}

type sipAckJob struct {
	mgr     *Manager
	ack     *pdu.IPv4Packet
	phoneIP pdu.IPAddress
}

func (m *Manager) findPhoneByIP(ip pdu.IPAddress) *VoIPPhone {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.VoIPPhones {
		if p.VoiceIP == ip || p.DataIP == ip {
			return p
		}
	}
	return nil
}

func (m *Manager) buildSIPReplyIP(cmIP pdu.IPAddress, req *pdu.IPv4Packet, reply *pdu.SIPPacket) *pdu.IPv4Packet {
	return &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoUDP,
		SourceIP: cmIP, DestinationIP: req.SourceIP,
		SrcPort: pdu.PortSIP, DstPort: req.SrcPort,
		DSCP: pdu.DSCPEF,
		SIP: reply,
	}
}

func (m *Manager) buildSCCPReplyIP(cmIP pdu.IPAddress, req *pdu.IPv4Packet, reply *pdu.SCCPPacket) *pdu.IPv4Packet {
	return &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoUDP,
		SourceIP: cmIP, DestinationIP: req.SourceIP,
		SrcPort: pdu.PortSCCP, DstPort: req.SrcPort,
		DSCP: pdu.DSCPEF,
		SCCP: reply,
	}
}

func (m *Manager) sendVoIPReply(nodeID, portID string, router *Router, host *Host, ip *pdu.IPv4Packet, simTime time.Duration) {
	if host != nil {
		m.sendIPFromHost(host, ip, simTime)
		return
	}
	if router != nil {
		m.forwardIPFromRouter(router, portID, ip, simTime, 1.0)
	}
}

func (m *Manager) deliverToVoIPPhone(phone *VoIPPhone, portID string, wire *pdu.WireFrame) {
	payload, err := pdu.DecodeFramePayload(wire.Frame)
	if err != nil || payload == nil || payload.IP == nil {
		return
	}
	ip := payload.IP
	if ip.Protocol != pdu.ProtoUDP {
		return
	}
	simTime := m.SimNow()
	if ip.SIP != nil && ip.SIP.Method == pdu.SIPRinging {
		m.LogEvent(EventProtocol, phone.ID, portID, "SIP ringing", nil)
	}
	if ip.SIP != nil && ip.SIP.Method == pdu.SIPOK {
		m.updateCallState(ip.SIP.CallID, CallConnected, simTime)
		m.startRTPStream(phone, ip, simTime)
	}
	if ip.SCCP != nil && ip.SCCP.Type == pdu.SCCPCallProceed {
		m.updateCallState(ip.SCCP.CallID, CallConnected, simTime)
	}
}

func (m *Manager) startRTPStream(phone *VoIPPhone, sipOK *pdu.IPv4Packet, simTime time.Duration) {
	if m.scheduler == nil {
		return
	}
	rtp := &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoUDP,
		SourceIP: phone.VoiceIP, DestinationIP: sipOK.SourceIP,
		SrcPort: pdu.PortRTP, DstPort: pdu.PortRTP,
		DSCP: pdu.DSCPEF,
		RTP: &pdu.RTPPacket{Codec: "g711ulaw", PayloadLen: 160, SSRC: 0x1234},
	}
	m.scheduler.Schedule(engine.EventTimerVoIP, 20*time.Millisecond, &rtpJob{mgr: m, phoneID: phone.ID, pkt: rtp})
}

type rtpJob struct {
	mgr     *Manager
	phoneID string
	pkt     *pdu.IPv4Packet
}

// SendSIPOnWire transmits a SIP packet from a phone through the dataplane.
func (m *Manager) SendSIPOnWire(phoneID string, pkt *pdu.SIPPacket) error {
	phone, ok := m.GetVoIPPhone(phoneID)
	if !ok {
		return errDeviceNotFound(phoneID)
	}
	if pkt.DestIP == "" {
		pkt.DestIP = phone.CallManagerIP
	}
	ip := &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoUDP,
		SourceIP: phone.VoiceIP, DestinationIP: pkt.DestIP,
		SrcPort: pdu.PortSIP, DstPort: pdu.PortSIP,
		DSCP: pdu.DSCPEF,
		SIP: pkt,
	}
	m.sendVoIPFromPhone(phone, ip)
	return nil
}

// SendSCCPOnWire transmits SCCP from a phone.
func (m *Manager) SendSCCPOnWire(phoneID string, pkt *pdu.SCCPPacket) error {
	phone, ok := m.GetVoIPPhone(phoneID)
	if !ok {
		return errDeviceNotFound(phoneID)
	}
	if pkt.DestIP == "" {
		pkt.DestIP = phone.CallManagerIP
	}
	pkt.SourceIP = phone.VoiceIP
	ip := &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoUDP,
		SourceIP: phone.VoiceIP, DestinationIP: pkt.DestIP,
		SrcPort: pdu.PortSCCP, DstPort: pdu.PortSCCP,
		DSCP: pdu.DSCPEF,
		SCCP: pkt,
	}
	m.sendVoIPFromPhone(phone, ip)
	return nil
}

func (m *Manager) sendVoIPFromPhone(phone *VoIPPhone, ip *pdu.IPv4Packet) {
	simTime := m.SimNow()
	if phone.UplinkNode != "" {
		if host, ok := m.GetHost(phone.UplinkNode); ok {
			m.sendIPFromHost(host, ip, simTime)
			return
		}
		if router, ok := m.GetRouter(phone.UplinkNode); ok {
			port := phone.PortID
			if port == "" {
				port = "GigabitEthernet0/0"
			}
			m.forwardIPFromRouter(router, port, ip, simTime, 1.0)
			return
		}
	}
	m.mu.RLock()
	for _, r := range m.Routers {
		m.mu.RUnlock()
		m.forwardIPFromRouter(r, "GigabitEthernet0/0", ip, simTime, 1.0)
		return
	}
	m.mu.RUnlock()
}
