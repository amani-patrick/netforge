package network

import (
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// SendOspfHelloOnWire encapsulates and transmits OSPF Hello via the data plane.
func (m *Manager) SendOspfHelloOnWire(router *Router, portID string, hello *protocol.OspfHelloPacket) {
	ifaceIP := router.Interfaces[portID]
	srcMAC := router.InterfaceMAC[portID]

	neighbors := make([]pdu.IPAddress, len(hello.ActiveNeighbors))
	copy(neighbors, hello.ActiveNeighbors)

	ospfWire := &pdu.OSPFWirePacket{
		Hello: &pdu.OspfHelloWire{
			RouterID: hello.RouterID, NetworkMask: hello.NetworkMask,
			ActiveNeighbors: neighbors,
		},
	}

	ipPkt := &pdu.IPv4Packet{
		Version: 4, TTL: 1, Protocol: pdu.ProtoOSPF,
		SourceIP: ifaceIP, DestinationIP: pdu.IPAddress(protocol.OspfMulticast),
	}

	frame := &pdu.EthernetFrame{
		DestinationMAC: pdu.MACOspfAllSPFRouters,
		SourceMAC:      srcMAC,
		EtherType:      pdu.TypeIPv4,
	}
	_ = pdu.EncodeFramePayload(frame, &pdu.FramePayload{Type: pdu.PayloadOSPF, OSPF: ospfWire, IP: ipPkt})

	wire := &pdu.WireFrame{Frame: frame, Physical: &pdu.L1Metadata{SourceNodeID: router.ID, SourcePortID: portID}}
	m.deliverProtocolFrame(router.ID, portID, wire)
}

// deliverProtocolFrame transmits a control-plane frame to a directly connected peer immediately.
func (m *Manager) deliverProtocolFrame(nodeID, portID string, wire *pdu.WireFrame) {
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
		wire.Physical.Bandwidth = meta.Bandwidth
		wire.Physical.CableLength = meta.CableLength
	}
	m.CaptureTX(nodeID, portID, wire)
	m.LogEvent(EventPacketTX, nodeID, portID, "protocol frame transmitted", map[string]interface{}{
		"dest_node": destNode, "dest_port": destPort,
	})
	m.DeliverFrame(destNode, destPort, wire)
}

// SendRipUpdateOnWire transmits RIP response over UDP/520 broadcast.
func (m *Manager) SendRipUpdateOnWire(router *Router, portID string, update *protocol.RipUpdate) {
	ifaceIP := router.Interfaces[portID]
	srcMAC := router.InterfaceMAC[portID]

	routes := make([]pdu.RIPWireRoute, 0, len(update.Routes))
	for _, r := range update.Routes {
		routes = append(routes, pdu.RIPWireRoute{Family: 2, CIDR: r.Network, Metric: uint16(r.Metric)})
	}
	ripWire := &pdu.RIPWirePacket{Command: 2, Routes: routes}

	ipPkt := &pdu.IPv4Packet{
		Version: 4, TTL: 1, Protocol: pdu.ProtoUDP,
		SourceIP: ifaceIP, DestinationIP: pdu.IPAddress("255.255.255.255"),
	}

	frame := &pdu.EthernetFrame{
		DestinationMAC: pdu.MACBroadcast,
		SourceMAC:      srcMAC,
		EtherType:      pdu.TypeIPv4,
	}
	_ = pdu.EncodeFramePayload(frame, &pdu.FramePayload{Type: pdu.PayloadRIP, RIP: ripWire, IP: ipPkt})

	wire := &pdu.WireFrame{Frame: frame, Physical: &pdu.L1Metadata{SourceNodeID: router.ID, SourcePortID: portID}}
	m.deliverProtocolFrame(router.ID, portID, wire)
}

// HandleWireOSPF processes on-wire OSPF packets.
func (m *Manager) HandleWireOSPF(router *Router, portID string, ospf *pdu.OSPFWirePacket) {
	if ospf.Hello == nil || router.Ospf == nil {
		return
	}
	helloPkt := &protocol.OspfHelloPacket{
		RouterID: ospf.Hello.RouterID, NetworkMask: ospf.Hello.NetworkMask,
		HelloInterval: 10, DeadInterval: 40,
		ActiveNeighbors: ospf.Hello.ActiveNeighbors,
	}
	router.Ospf.HandleIncomingHello(portID, helloPkt, m.SimNow())
	m.LogEvent(EventOSPF, router.ID, portID, "OSPF hello received", nil)
}

// HandleWireRIP processes on-wire RIP packets.
func (m *Manager) HandleWireRIP(router *Router, portID string, rip *pdu.RIPWirePacket, senderIP pdu.IPAddress) {
	if router.Rip == nil || !router.Rip.Enabled {
		return
	}
	entries := make([]protocol.RipRouteEntry, 0, len(rip.Routes))
	for _, r := range rip.Routes {
		entries = append(entries, protocol.RipRouteEntry{Network: r.CIDR, Metric: int(r.Metric)})
	}
	update := &protocol.RipUpdate{Routes: entries}
	newRoutes := router.Rip.ProcessUpdate(update, senderIP, portID, m.SimNow())
	for _, route := range newRoutes {
		_ = router.AddRoute(route.Network, route.NextHop, route.Interface, route.Metric, protocol.RouteRIP, protocol.AdminDistRIP)
	}
	m.LogEvent(EventRIP, router.ID, portID, "RIP update processed", map[string]interface{}{
		"routes": len(newRoutes),
	})
}
