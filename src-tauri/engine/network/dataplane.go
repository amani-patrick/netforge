package network

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"netforge/engine"
	"netforge/engine/pdu"
)

// PacketDelivery is scheduled when a frame arrives at a destination port.
type PacketDelivery struct {
	DestNodeID string         `json:"dest_node_id"`
	DestPortID string         `json:"dest_port_id"`
	WireFrame  *pdu.WireFrame `json:"wire_frame"`
}

// PingSession tracks an in-flight ICMP echo request.
type PingSession struct {
	ID        string
	SourceID  string
	DestIP    string
	ICMPID    uint16
	Sequence  uint16
	SentAt    time.Duration
	ReplyConn ReplyTarget
}

// ReplyTarget delivers async results back to a WebSocket client.
type ReplyTarget struct {
	RequestID string
}

// PingResult is sent to the UI when a ping completes or times out.
type PingResult struct {
	SourceID  string  `json:"source_id"`
	DestIP    string  `json:"dest_ip"`
	Success   bool    `json:"success"`
	RTT       float64 `json:"rtt_ms"`
	Message   string  `json:"message"`
	RequestID string  `json:"request_id,omitempty"`
}

var (
	frameCounter uint64
	frameCounterMu sync.Mutex
)

func nextFrameID() string {
	frameCounterMu.Lock()
	defer frameCounterMu.Unlock()
	frameCounter++
	return fmt.Sprintf("frame_%d", frameCounter)
}

// SetScheduler attaches the discrete-event scheduler to the manager.
func (m *Manager) SetScheduler(s *engine.Scheduler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduler = s
}

// SimNow returns the current simulation clock.
func (m *Manager) SimNow() time.Duration {
	if m.scheduler == nil {
		return 0
	}
	return m.scheduler.CurrentTime()
}

// HandleSimulationEvent dispatches scheduler events to the data plane.
func (m *Manager) HandleSimulationEvent(event *engine.Event) {
	switch event.Type {
	case engine.EventPacketRx:
		if delivery, ok := event.Data.(*PacketDelivery); ok {
			m.DeliverFrame(delivery.DestNodeID, delivery.DestPortID, delivery.WireFrame)
		}
	case engine.EventTimerICMP:
		if sessionID, ok := event.Data.(string); ok {
			m.handlePingTimeout(sessionID)
		}
	case engine.EventTimerVoIP:
		m.handleVoIPTimer(event.Data)
	}
}

func (m *Manager) handleVoIPTimer(data interface{}) {
	switch job := data.(type) {
	case *sipAckJob:
		phone := m.findPhoneByIP(job.phoneIP)
		if phone != nil {
			m.sendVoIPFromPhone(phone, job.ack)
			if job.ack.SIP != nil {
				m.updateCallState(job.ack.SIP.CallID, CallConnected, m.SimNow())
			}
		}
	case *rtpJob:
		phone, ok := m.GetVoIPPhone(job.phoneID)
		if !ok {
			return
		}
		m.sendVoIPFromPhone(phone, job.pkt)
	}
}

// TransmitFrame schedules a frame for delivery across a link.
func (m *Manager) TransmitFrame(wire *pdu.WireFrame) {
	if m.scheduler == nil || wire.Physical == nil {
		return
	}
	if wire.ID == "" {
		wire.ID = nextFrameID()
	}

	delay := calcTransmissionDelay(wire)
	delivery := &PacketDelivery{
		DestNodeID: wire.Physical.DestNodeID,
		DestPortID: wire.Physical.DestPortID,
		WireFrame:  wire,
	}
	m.scheduler.Schedule(engine.EventPacketRx, delay, delivery)
}

func calcTransmissionDelay(wire *pdu.WireFrame) time.Duration {
	meta := wire.Physical
	bandwidth := meta.Bandwidth
	if bandwidth <= 0 {
		bandwidth = engine.DefaultBandwidth
	}
	cableLen := meta.CableLength
	if cableLen <= 0 {
		cableLen = engine.DefaultCableLength
	}

	frameBits := int64(64 * 8) // minimum Ethernet frame
	if wire.Frame != nil && len(wire.Frame.Payload) > 0 {
		frameBits = int64((14 + len(wire.Frame.Payload)) * 8)
	}

	serialization := time.Duration(frameBits) * time.Second / time.Duration(bandwidth)
	propagation := time.Duration(cableLen*5) * time.Microsecond
	delay := serialization + propagation
	if meta.DelayMultiplier > 1 {
		delay = time.Duration(float64(delay) * meta.DelayMultiplier)
	}
	if wire.QoSPriority >= 1 {
		return delay / 4
	}
	return delay
}

// DeliverFrame hands a received frame to the correct device.
func (m *Manager) DeliverFrame(nodeID, portID string, wire *pdu.WireFrame) {
	m.CaptureRX(nodeID, portID, wire)
	m.LogEvent(EventPacketRX, nodeID, portID, "frame received", map[string]interface{}{"frame_id": wire.ID})

	if ap, ok := m.GetAccessPoint(nodeID); ok {
		m.deliverToAP(ap, portID, wire)
		return
	}
	if asa, ok := m.GetASAFirewall(nodeID); ok {
		m.deliverToASA(asa, portID, wire)
		return
	}
	if router, ok := m.GetRouter(nodeID); ok {
		m.deliverToRouter(router, portID, wire)
		return
	}
	if sw, ok := m.GetSwitch(nodeID); ok {
		m.deliverToSwitch(sw, portID, wire)
		return
	}
	if host, ok := m.GetHost(nodeID); ok {
		m.deliverToHost(host, portID, wire)
		return
	}
	if phone, ok := m.GetVoIPPhone(nodeID); ok {
		m.deliverToVoIPPhone(phone, portID, wire)
	}
}

func (m *Manager) deliverToSwitch(sw *Switch, portID string, wire *pdu.WireFrame) {
	simTime := m.SimNow()
	outbound := sw.HandleIncomingFrame(portID, wire, simTime)
	for outPort, outFrame := range outbound {
		if payload, err := pdu.DecodeFramePayload(outFrame.Frame); err == nil && payload != nil && payload.IP != nil {
			outFrame.QoSPriority = sw.ClassifySwitchFrame(outPort, payload.IP.DSCP)
		}
		m.forwardFromDevice(sw.ID, outPort, outFrame)
	}
}

func (m *Manager) deliverToHost(host *Host, portID string, wire *pdu.WireFrame) {
	simTime := m.SimNow()
	payload, err := pdu.DecodeFramePayload(wire.Frame)
	if err != nil || payload == nil {
		return
	}

	if payload.ARP != nil {
		m.handleHostARP(host, portID, wire, payload.ARP, simTime)
		return
	}

	if payload.IP != nil {
		m.handleHostIP(host, portID, wire, payload.IP, simTime)
		return
	}

	if payload.IPv6 != nil {
		m.handleHostIPv6(host, portID, wire, payload.IPv6, simTime)
		return
	}

	if payload.NDP != nil {
		m.handleHostNDP(host, portID, payload.NDP, simTime)
	}
}

func (m *Manager) handleHostARP(host *Host, portID string, wire *pdu.WireFrame, arp *pdu.ARPPacket, simTime time.Duration) {
	host.LearnARP(arp.SenderIP, arp.SenderMAC, simTime)

	if arp.Operation == pdu.ArpRequest && host.OwnsIP(arp.TargetIP) {
		replyArp := &pdu.ARPPacket{
			HardwareType: 1,
			ProtocolType: 0x0800,
			Operation:    pdu.ArpReply,
			SenderMAC:    host.MAC,
			SenderIP:     host.IP,
			TargetMAC:    arp.SenderMAC,
			TargetIP:     arp.SenderIP,
		}
		frame, err := pdu.NewARPFrame(arp.SenderMAC, host.MAC, replyArp)
		if err == nil {
			m.forwardFromDevice(host.ID, host.PortID, m.wrapWireFrame(host.ID, host.PortID, frame))
		}
		return
	}

	if arp.Operation == pdu.ArpReply {
		queued := host.DequeuePackets(arp.SenderIP)
		for _, pkt := range queued {
			m.sendIPFromHost(host, pkt, simTime)
		}
	}
}

func (m *Manager) handleHostIP(host *Host, portID string, wire *pdu.WireFrame, ip *pdu.IPv4Packet, simTime time.Duration) {
	if !host.OwnsIP(ip.DestinationIP) {
		return
	}

	if ip.Protocol == pdu.ProtoICMP && ip.ICMP != nil {
		if ip.ICMP.Type == pdu.ICMPEchoRequest {
			replyIP := &pdu.IPv4Packet{
				Version:       4,
				TTL:           64,
				Protocol:      pdu.ProtoICMP,
				SourceIP:      host.IP,
				DestinationIP: ip.SourceIP,
				ICMP:          pdu.NewEchoReply(ip.ICMP),
			}
			m.sendIPFromHost(host, replyIP, simTime)
		} else if ip.ICMP.Type == pdu.ICMPEchoReply {
			m.completePing(host.ID, ip.SourceIP, ip.ICMP.ID, ip.ICMP.Sequence, simTime)
		}
		return
	}
	if m.handleLocalIPv4(host.ID, host.PortID, nil, host, ip, simTime) {
		return
	}
}

func (m *Manager) deliverToRouter(router *Router, portID string, wire *pdu.WireFrame) {
	if !router.IsInterfaceUp(portID) {
		return
	}
	simTime := m.SimNow()
	payload, err := pdu.DecodeFramePayload(wire.Frame)
	if err != nil || payload == nil {
		return
	}

	if payload.CDP != nil {
		router.LearnCDPNeighbor(portID, payload.CDP)
		return
	}

	if payload.DHCP != nil {
		m.handleRouterDHCP(router, portID, payload.DHCP, simTime)
		return
	}

	if payload.ARP != nil {
		m.handleRouterARP(router, portID, wire, payload.ARP, simTime)
		return
	}

	if payload.OSPF != nil {
		m.HandleWireOSPF(router, portID, payload.OSPF)
		return
	}

	if payload.RIP != nil {
		senderIP := pdu.IPAddress("")
		if payload.IP != nil {
			senderIP = payload.IP.SourceIP
		}
		m.HandleWireRIP(router, portID, payload.RIP, senderIP)
		return
	}

	if payload.IP != nil {
		m.handleRouterIP(router, portID, wire, payload.IP, simTime)
		return
	}

	if payload.IPv6 != nil {
		m.handleRouterIPv6(router, portID, payload.IPv6, simTime)
		return
	}

	if payload.NDP != nil {
		replyWire, queued := router.HandleNDP(portID, payload.NDP, simTime)
		if replyWire != nil {
			m.forwardFromDevice(router.ID, portID, replyWire)
		}
		for _, pkt := range queued {
			m.forwardIPv6FromRouter(router, pkt, simTime)
		}
		return
	}

	if payload.PPP != nil {
		m.handleRouterPPP(router, portID, payload.PPP, simTime)
		return
	}

	if payload.FR != nil {
		m.handleRouterFR(router, portID, wire, payload.FR)
	}
}

func (m *Manager) handleRouterDHCP(router *Router, portID string, dhcp *pdu.DHCPPacket, simTime time.Duration) {
	var reply *pdu.DHCPPacket
	switch dhcp.MessageType {
	case pdu.DHCPDiscover:
		reply = router.HandleDHCPDiscover(dhcp, portID)
	case pdu.DHCPRequest:
		reply = router.HandleDHCPRequest(dhcp, portID)
	}
	if reply == nil {
		return
	}
	frame := &pdu.EthernetFrame{
		DestinationMAC: pdu.MACBroadcast,
		SourceMAC:      router.InterfaceMAC[portID],
	}
	_ = pdu.EncodeFramePayload(frame, &pdu.FramePayload{Type: pdu.PayloadDHCP, DHCP: reply})
	m.forwardFromDevice(router.ID, portID, m.wrapWireFrame(router.ID, portID, frame))
	_ = simTime
}

func (m *Manager) handleRouterARP(router *Router, portID string, wire *pdu.WireFrame, arp *pdu.ARPPacket, simTime time.Duration) {
	replyWire, queued := router.HandleIncomingArp(portID, arp, simTime)
	if replyWire != nil {
		m.forwardFromDevice(router.ID, portID, replyWire)
	}
	for _, pkt := range queued {
		m.forwardIPFromRouter(router, portID, pkt, simTime, 1.0)
	}
}

func (m *Manager) handleRouterIP(router *Router, portID string, wire *pdu.WireFrame, ip *pdu.IPv4Packet, simTime time.Duration) {
	ip = router.TranslateInbound(portID, ip)
	if !router.EvaluateACL(router.GetInboundACL(portID), ip) {
		m.LogEvent(EventACLDeny, router.ID, portID, "inbound ACL deny", map[string]interface{}{
			"src": string(ip.SourceIP), "dst": string(ip.DestinationIP),
		})
		return
	}

	if router.OwnsIP(ip.DestinationIP) {
		if ip.Protocol == pdu.ProtoESP && ip.ESP != nil {
			if inner := router.DecapsulateESP(ip); inner != nil {
				m.LogEvent(EventProtocol, router.ID, portID, "IPsec decrypt", map[string]interface{}{
					"src": string(inner.SourceIP), "dst": string(inner.DestinationIP),
				})
				m.forwardIPFromRouter(router, portID, inner, simTime, 1.0)
			}
			return
		}
		if m.handleLocalIPv4(router.ID, portID, router, nil, ip, simTime) {
			return
		}
		if ip.Protocol == pdu.ProtoICMP && ip.ICMP != nil && ip.ICMP.Type == pdu.ICMPEchoRequest {
			replyIP := &pdu.IPv4Packet{
				Version: 4, TTL: 64, Protocol: pdu.ProtoICMP,
				SourceIP: ip.DestinationIP, DestinationIP: ip.SourceIP,
				ICMP: pdu.NewEchoReply(ip.ICMP),
			}
			m.forwardIPFromRouter(router, portID, replyIP, simTime, 1.0)
		}
		return
	}

	if ip.TTL <= 1 {
		return
	}
	dscp, drop := router.ApplyQoSToPacket(portID, ip, simTime)
	if drop {
		m.LogEvent(EventPacketRX, router.ID, portID, "QoS police drop", nil)
		return
	}
	ip.DSCP = dscp
	bwShare := router.QoSBandwidthShare(portID, ip)
	ip.TTL--
	m.forwardIPFromRouter(router, portID, ip, simTime, bwShare)
}

func (m *Manager) forwardIPFromRouter(router *Router, inPort string, ip *pdu.IPv4Packet, simTime time.Duration, bwShare float64) {
	route, found := router.MatchRoute(ip.DestinationIP)
	if !found {
		return
	}

	nextHop := ip.DestinationIP
	if route.NextHop != "" {
		nextHop = route.NextHop
	}

	outPort := route.Interface
	if !router.IsInterfaceUp(outPort) {
		return
	}

	outIP := router.TranslateOutbound(outPort, ip)
	outACL := router.GetOutboundACL(outPort)
	if !router.EvaluateACL(outACL, outIP) {
		m.LogEvent(EventACLDeny, router.ID, outPort, "outbound ACL deny", map[string]interface{}{
			"src": string(outIP.SourceIP), "dst": string(outIP.DestinationIP),
		})
		return
	}

	if peer, encrypt := router.MatchCryptoTunnel(outPort, outIP); encrypt {
		esp := router.EncapsulateESP(outIP, peer, outPort)
		m.LogEvent(EventProtocol, router.ID, outPort, "IPsec encrypt", map[string]interface{}{
			"peer": string(peer.PeerIP), "dst": string(outIP.DestinationIP),
		})
		outIP = esp
		nextHop = peer.PeerIP
	}

	dstMAC, ok := router.LookupARP(nextHop, simTime)
	if !ok {
		router.QueuePacket(nextHop, outIP)
		arpReq := router.BuildARPRequest(outPort, nextHop)
		if arpReq != nil {
			m.forwardFromDevice(router.ID, outPort, arpReq)
		}
		return
	}

	srcMAC := router.InterfaceMAC[outPort]
	frame, err := pdu.NewIPv4Frame(dstMAC, srcMAC, outIP)
	if err != nil {
		return
	}
	wf := m.wrapWireFrame(router.ID, outPort, frame)
	wf.QoSPriority = qosWirePriority(outIP)
	if bwShare > 1 && wf.Physical == nil {
		wf.Physical = &pdu.L1Metadata{}
	}
	if bwShare > 1 {
		wf.Physical.DelayMultiplier = bwShare
	}
	m.forwardFromDevice(router.ID, outPort, wf)
}

func qosWirePriority(ip *pdu.IPv4Packet) int {
	if ip.DSCP >= pdu.DSCPEF {
		return 1
	}
	if ip.Protocol == pdu.ProtoUDP && (ip.DstPort == pdu.PortSIP || ip.DstPort == pdu.PortRTP) {
		return 1
	}
	return 0
}

func (m *Manager) sendIPFromHost(host *Host, ip *pdu.IPv4Packet, simTime time.Duration) {
	nextHopIP := host.ResolveNextHopIP(ip.DestinationIP)
	dstMAC, ok := host.LookupARP(nextHopIP, simTime)
	if !ok {
		host.QueuePacket(nextHopIP, ip)
		arpFrame := m.buildHostARPRequest(host, nextHopIP)
		if arpFrame != nil {
			m.forwardFromDevice(host.ID, host.PortID, arpFrame)
		}
		return
	}

	frame, err := pdu.NewIPv4Frame(dstMAC, host.MAC, ip)
	if err != nil {
		return
	}
	wf := m.wrapWireFrame(host.ID, host.PortID, frame)
	wf.QoSPriority = qosWirePriority(ip)
	m.forwardFromDevice(host.ID, host.PortID, wf)
}

func (m *Manager) buildHostARPRequest(host *Host, targetIP pdu.IPAddress) *pdu.WireFrame {
	arp := &pdu.ARPPacket{
		HardwareType: 1,
		ProtocolType: 0x0800,
		Operation:    pdu.ArpRequest,
		SenderMAC:    host.MAC,
		SenderIP:     host.IP,
		TargetMAC:    pdu.MACBroadcast,
		TargetIP:     targetIP,
	}
	frame, err := pdu.NewARPFrame(pdu.MACBroadcast, host.MAC, arp)
	if err != nil {
		return nil
	}
	return m.wrapWireFrame(host.ID, host.PortID, frame)
}

func (m *Manager) wrapWireFrame(nodeID, portID string, frame *pdu.EthernetFrame) *pdu.WireFrame {
	return &pdu.WireFrame{
		ID:    nextFrameID(),
		Frame: frame,
		Physical: &pdu.L1Metadata{
			SourceNodeID: nodeID,
			SourcePortID: portID,
		},
	}
}

// forwardFromDevice transmits a frame out of a device port across the topology link.
func (m *Manager) forwardFromDevice(nodeID, portID string, wire *pdu.WireFrame) {
	if router, ok := m.GetRouter(nodeID); ok {
		if !router.IsInterfaceUp(portID) {
			return
		}
	}
	destNode, destPort, meta, ok := m.ResolveLinkPeer(nodeID, portID)
	if !ok {
		return
	}
	if wire.Physical == nil {
		wire.Physical = &pdu.L1Metadata{}
	}
	wire.Physical.SourceNodeID = nodeID
	wire.Physical.SourcePortID = portID
	wire.Physical.DestNodeID = destNode
	wire.Physical.DestPortID = destPort
	if meta != nil {
		if meta.Bandwidth > 0 {
			wire.Physical.Bandwidth = meta.Bandwidth
		}
		if meta.CableLength > 0 {
			wire.Physical.CableLength = meta.CableLength
		}
	}
	m.CaptureTX(nodeID, portID, wire)
	m.LogEvent(EventPacketTX, nodeID, portID, "frame transmitted", map[string]interface{}{
		"frame_id": wire.ID, "dest_node": destNode, "dest_port": destPort,
	})
	m.TransmitFrame(wire)
}

// StartPing initiates an ICMP echo from a host to a destination IP.
func (m *Manager) StartPing(sourceID, destIP, requestID string) error {
	host, ok := m.GetHost(sourceID)
	if !ok {
		return fmt.Errorf("host %s not found", sourceID)
	}

	simTime := m.SimNow()
	sessionID := fmt.Sprintf("ping_%s_%d", sourceID, simTime)
	icmpID := uint16(simTime/time.Millisecond) % 65535
	seq := uint16(1)

	m.mu.Lock()
	if m.pingSessions == nil {
		m.pingSessions = make(map[string]*PingSession)
	}
	m.pingSessions[sessionID] = &PingSession{
		ID:        sessionID,
		SourceID:  sourceID,
		DestIP:    destIP,
		ICMPID:    icmpID,
		Sequence:  seq,
		SentAt:    simTime,
		ReplyConn: ReplyTarget{RequestID: requestID},
	}
	m.mu.Unlock()

	echo := pdu.NewEchoRequest(icmpID, seq, []byte("NetForge ping"))
	ipPkt := &pdu.IPv4Packet{
		Version:       4,
		TTL:           64,
		Protocol:      pdu.ProtoICMP,
		SourceIP:      host.IP,
		DestinationIP: pdu.IPAddress(destIP),
		ICMP:          echo,
	}
	m.sendIPFromHost(host, ipPkt, simTime)

	if m.scheduler != nil {
		m.scheduler.Schedule(engine.EventTimerICMP, engine.IcmpPingTimeout, sessionID)
	}
	return nil
}

func (m *Manager) completePing(hostID string, replyFrom pdu.IPAddress, icmpID, seq uint16, simTime time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, session := range m.pingSessions {
		if session.SourceID != hostID || session.ICMPID != icmpID || session.Sequence != seq {
			continue
		}
		if string(replyFrom) != session.DestIP {
			continue
		}
		rtt := float64(simTime-session.SentAt) / float64(time.Millisecond)
		m.pendingPingResults = append(m.pendingPingResults, PingResult{
			SourceID:  session.SourceID,
			DestIP:    session.DestIP,
			Success:   true,
			RTT:       rtt,
			Message:   fmt.Sprintf("Reply from %s: bytes=32 time=%.0fms", session.DestIP, rtt),
			RequestID: session.ReplyConn.RequestID,
		})
		delete(m.pingSessions, id)
		return
	}
}

func (m *Manager) handlePingTimeout(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.pingSessions[sessionID]
	if !ok {
		return
	}
	m.pendingPingResults = append(m.pendingPingResults, PingResult{
		SourceID:  session.SourceID,
		DestIP:    session.DestIP,
		Success:   false,
		Message:   "Request timed out.",
		RequestID: session.ReplyConn.RequestID,
	})
	delete(m.pingSessions, sessionID)
}

// DrainPingResults returns and clears pending ping results for IPC.
func (m *Manager) DrainPingResults() []PingResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	results := m.pendingPingResults
	m.pendingPingResults = nil
	return results
}

// SaveTopology writes the current simulation state to a JSON file.
func (m *Manager) SaveTopology(path string) error {
	snap := m.ExportSnapshot()
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadTopology restores simulation state from a JSON file.
func (m *Manager) LoadTopology(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var snap TopologySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	m.ImportSnapshot(snap)
	return nil
}

func (m *Manager) deliverToAP(ap *AccessPoint, portID string, wire *pdu.WireFrame) {
	_ = portID
	if wire.Frame == nil {
		return
	}
	m.forwardFromDevice(ap.ID, "Dot11Radio0", wire)
}

func (m *Manager) deliverToASA(asa *ASAFirewall, portID string, wire *pdu.WireFrame) {
	simTime := m.SimNow()
	payload, err := pdu.DecodeFramePayload(wire.Frame)
	if err != nil || payload == nil {
		return
	}
	if payload.IP == nil {
		return
	}
	if !asa.InspectPacket(portID, payload.IP, simTime) {
		return
	}
	if asa.OwnsIP(payload.IP.DestinationIP) {
		return
	}
	outPort, ok := asa.MatchRoute(payload.IP.DestinationIP)
	if !ok || outPort == portID {
		return
	}
	payload.IP.TTL--
	frame, err := pdu.NewIPv4Frame(pdu.MACBroadcast, asa.InterfaceMAC[outPort], payload.IP)
	if err != nil {
		return
	}
	m.forwardFromDevice(asa.ID, outPort, m.wrapWireFrame(asa.ID, outPort, frame))
}

func (m *Manager) handleRouterIPv6(router *Router, portID string, ip *pdu.IPv6Packet, simTime time.Duration) {
	if router.OwnsIPv6(ip.DestinationIP) {
		if ip.NextHeader == pdu.ProtoICMPv6 && ip.ICMPv6 != nil && ip.ICMPv6.Type == pdu.ICMPEchoRequest {
			reply := &pdu.IPv6Packet{
				HopLimit: 64, NextHeader: pdu.ProtoICMPv6,
				SourceIP: ip.DestinationIP, DestinationIP: ip.SourceIP,
				ICMPv6: pdu.NewEchoReply(ip.ICMPv6),
			}
			m.forwardIPv6FromRouter(router, reply, simTime)
		}
		return
	}
	if ip.HopLimit <= 1 {
		return
	}
	ip.HopLimit--
	m.forwardIPv6FromRouter(router, ip, simTime)
}

func (m *Manager) forwardIPv6FromRouter(router *Router, ip *pdu.IPv6Packet, simTime time.Duration) {
	route, found := router.MatchIPv6Route(ip.DestinationIP)
	if !found {
		return
	}
	nextHop := ip.DestinationIP
	if route.NextHop != "" {
		nextHop = route.NextHop
	}
	outPort := route.Interface
	if !router.IsInterfaceUp(outPort) {
		return
	}
	dstMAC, ok := router.LookupNDP(nextHop, simTime)
	if !ok {
		router.QueueIPv6Packet(nextHop, ip)
		solicit := router.BuildNDPSolicit(outPort, nextHop)
		if solicit != nil {
			m.forwardFromDevice(router.ID, outPort, solicit)
		}
		return
	}
	srcMAC := router.InterfaceMAC[outPort]
	frame, err := pdu.NewIPv6Frame(dstMAC, srcMAC, ip)
	if err != nil {
		return
	}
	m.forwardFromDevice(router.ID, outPort, m.wrapWireFrame(router.ID, outPort, frame))
}

func (m *Manager) sendIPv6FromHost(host *Host, ip *pdu.IPv6Packet, simTime time.Duration) {
	nextHop := host.ResolveNextHopIPv6(ip.DestinationIP)
	dstMAC, ok := host.LookupNDP(nextHop, simTime)
	if !ok {
		host.QueueIPv6Packet(nextHop, ip)
		return
	}
	frame, err := pdu.NewIPv6Frame(dstMAC, host.MAC, ip)
	if err != nil {
		return
	}
	m.forwardFromDevice(host.ID, host.PortID, m.wrapWireFrame(host.ID, host.PortID, frame))
}

func (m *Manager) handleHostIPv6(host *Host, portID string, wire *pdu.WireFrame, ip *pdu.IPv6Packet, simTime time.Duration) {
	if !host.OwnsIPv6(ip.DestinationIP) {
		return
	}
	if ip.NextHeader == pdu.ProtoICMPv6 && ip.ICMPv6 != nil {
		if ip.ICMPv6.Type == pdu.ICMPEchoRequest {
			reply := &pdu.IPv6Packet{
				HopLimit: 64, NextHeader: pdu.ProtoICMPv6,
				SourceIP: host.IPv6, DestinationIP: ip.SourceIP,
				ICMPv6: pdu.NewEchoReply(ip.ICMPv6),
			}
			m.sendIPv6FromHost(host, reply, simTime)
		} else if ip.ICMPv6.Type == pdu.ICMPEchoReply {
			m.completePing(host.ID, pdu.IPAddress(ip.SourceIP), ip.ICMPv6.ID, ip.ICMPv6.Sequence, simTime)
		}
	}
	_ = portID
	_ = wire
}

func (m *Manager) handleHostNDP(host *Host, portID string, ndp *pdu.NDPPacket, simTime time.Duration) {
	if ndp.SenderIP != "" && ndp.SenderMAC != "" {
		host.LearnNDP(ndp.SenderIP, ndp.SenderMAC, simTime)
	}
	if ndp.Type == pdu.NDPNeighborAdvert {
		queued := host.DequeueIPv6(ndp.SenderIP)
		for _, pkt := range queued {
			m.sendIPv6FromHost(host, pkt, simTime)
		}
	}
	_ = portID
}

func (m *Manager) handleRouterPPP(router *Router, portID string, ppp *pdu.PPPFrame, simTime time.Duration) {
	reply := m.WAN.ProcessPPPFrame(router.ID, portID, ppp)
	if reply != nil {
		frame := &pdu.EthernetFrame{SourceMAC: router.InterfaceMAC[portID]}
		_ = pdu.EncodeFramePayload(frame, &pdu.FramePayload{Type: pdu.PayloadPPP, PPP: reply})
		destNode, destPort, _, ok := m.ResolveLinkPeer(router.ID, portID)
		if ok {
			m.DeliverFrame(destNode, destPort, &pdu.WireFrame{Frame: frame})
		}
	}
	if ppp.Stage == "DATA" && len(ppp.Payload) > 0 {
		router.AssignPPPSerialAddress(portID, len(m.Links), true)
	}
	_ = simTime
}

func (m *Manager) handleRouterFR(router *Router, portID string, wire *pdu.WireFrame, fr *pdu.FrameRelayFrame) {
	targetNode, targetPort, ok := m.WAN.ResolveFRDLCI(router.ID, portID, fr.DLCI)
	if !ok {
		return
	}
	wire.Physical = &pdu.L1Metadata{DestNodeID: targetNode, DestPortID: targetPort}
	m.DeliverFrame(targetNode, targetPort, wire)
}
