package network

import (
	"net"
	"sync"

	"netforge/engine"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// TopologyLink connects two device ports in the simulated network.
type TopologyLink struct {
	ID           string  `json:"id"`
	SourceNodeID string  `json:"source_node_id"`
	SourcePortID string  `json:"source_port_id"`
	TargetNodeID string  `json:"target_node_id"`
	TargetPortID string  `json:"target_port_id"`
	CableLength  float64 `json:"cable_length,omitempty"`
	Bandwidth    int64   `json:"bandwidth,omitempty"`
}

// Manager orchestrates all devices, links, and the data plane.
type Manager struct {
	Routers            map[string]*Router
	Switches           map[string]*Switch
	Hosts              map[string]*Host
	AccessPoints       map[string]*AccessPoint
	ASAFirewalls       map[string]*ASAFirewall
	VoIPPhones         map[string]*VoIPPhone
	CallManagers       map[string]*CallManager
	CellularGateways   map[string]*CellularGateway
	MobileUEs          map[string]*MobileUE
	WAN                *WANManager
	Links              []TopologyLink
	scheduler          *engine.Scheduler
	pingSessions       map[string]*PingSession
	pendingPingResults []PingResult
	eventLog           *EventLog
	pcap               *PortCapture
	activity           *ActivityEngine
	configStore        *ConfigStore
	signaling          *SignalingEngine
	mu                 sync.RWMutex
}

// NewManager creates an empty network topology manager.
func NewManager() *Manager {
	return &Manager{
		Routers:      make(map[string]*Router),
		Switches:     make(map[string]*Switch),
		Hosts:        make(map[string]*Host),
		AccessPoints: make(map[string]*AccessPoint),
		ASAFirewalls: make(map[string]*ASAFirewall),
		VoIPPhones:   make(map[string]*VoIPPhone),
		CallManagers: make(map[string]*CallManager),
		CellularGateways: make(map[string]*CellularGateway),
		MobileUEs:    make(map[string]*MobileUE),
		WAN:          NewWANManager(),
		Links:        make([]TopologyLink, 0),
		pingSessions: make(map[string]*PingSession),
		eventLog:     NewEventLog(10000),
		pcap:         NewPortCapture(500),
		activity:     NewActivityEngine(),
		configStore:  NewConfigStore(),
		signaling:    NewSignalingEngine(),
	}
}

// AddRouter provisions a virtual router in the topology.
func (m *Manager) AddRouter(id string) *Router {
	m.mu.Lock()
	defer m.mu.Unlock()
	router := NewRouter(id)
	m.Routers[id] = router
	return router
}

// AddSwitch provisions a virtual switch.
func (m *Manager) AddSwitch(id string) *Switch {
	m.mu.Lock()
	defer m.mu.Unlock()
	sw := NewSwitch(id)
	m.Switches[id] = sw
	return sw
}

// AddHost provisions an end-station host.
func (m *Manager) AddHost(id string) *Host {
	m.mu.Lock()
	defer m.mu.Unlock()
	host := NewHost(id)
	m.Hosts[id] = host
	return host
}

// GetRouter returns a router by ID.
func (m *Manager) GetRouter(id string) (*Router, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.Routers[id]
	return r, ok
}

// GetSwitch returns a switch by ID.
func (m *Manager) GetSwitch(id string) (*Switch, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sw, ok := m.Switches[id]
	return sw, ok
}

// GetHost returns a host by ID.
func (m *Manager) GetHost(id string) (*Host, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.Hosts[id]
	return h, ok
}

// AddLink connects two device ports.
func (m *Manager) AddLink(link TopologyLink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Links = append(m.Links, link)
	m.applyLinkState(link)
}

func (m *Manager) applyLinkState(link TopologyLink) {
	if sw, ok := m.Switches[link.SourceNodeID]; ok {
		sw.RegisterPort(link.SourcePortID)
	}
	if sw, ok := m.Switches[link.TargetNodeID]; ok {
		sw.RegisterPort(link.TargetPortID)
	}

	srcRouter, srcIsRouter := m.Routers[link.SourceNodeID]
	dstRouter, dstIsRouter := m.Routers[link.TargetNodeID]

	if srcIsRouter && dstIsRouter {
		linkIndex := len(m.Links)
		srcRouter.AssignLinkAddress(link.SourcePortID, linkIndex, true)
		dstRouter.AssignLinkAddress(link.TargetPortID, linkIndex, false)

		srcIP := srcRouter.Interfaces[link.SourcePortID]
		dstIP := dstRouter.Interfaces[link.TargetPortID]
		srcRouter.SetNeighbor(link.SourcePortID, link.TargetNodeID, link.TargetPortID, dstIP)
		dstRouter.SetNeighbor(link.TargetPortID, link.SourceNodeID, link.SourcePortID, srcIP)

		if srcRouter.Rip != nil && srcRouter.Rip.Enabled && dstIP != "" {
			srcRouter.Rip.AddNeighbor(link.SourcePortID, dstIP)
		}
		if dstRouter.Rip != nil && dstRouter.Rip.Enabled && srcIP != "" {
			dstRouter.Rip.AddNeighbor(link.TargetPortID, srcIP)
		}

		// Auto-complete IKE handshake if both ends already have matching crypto-maps + PSKs.
		go m.TryAutoNegotiateIKE(link.SourceNodeID, link.TargetNodeID)
	}

	if host, ok := m.Hosts[link.SourceNodeID]; ok {
		host.SetUplink(link.TargetNodeID)
	}
	if host, ok := m.Hosts[link.TargetNodeID]; ok {
		host.SetUplink(link.SourceNodeID)
	}
}

// ResolveLinkPeer finds the device on the other end of a cable.
func (m *Manager) ResolveLinkPeer(nodeID, portID string) (destNode, destPort string, meta *pdu.L1Metadata, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, link := range m.Links {
		if link.SourceNodeID == nodeID && link.SourcePortID == portID {
			meta = &pdu.L1Metadata{
				Bandwidth:   link.Bandwidth,
				CableLength: link.CableLength,
			}
			return link.TargetNodeID, link.TargetPortID, meta, true
		}
		if link.TargetNodeID == nodeID && link.TargetPortID == portID {
			meta = &pdu.L1Metadata{
				Bandwidth:   link.Bandwidth,
				CableLength: link.CableLength,
			}
			return link.SourceNodeID, link.SourcePortID, meta, true
		}
	}
	return "", "", nil, false
}

// AddStaticRoute adds a static route to a router.
func (m *Manager) AddStaticRoute(routerID, cidr, nextHop, iface string, metric int) error {
	router, ok := m.GetRouter(routerID)
	if !ok {
		return errDeviceNotFound(routerID)
	}
	err := router.AddRoute(cidr, pdu.IPAddress(nextHop), iface, metric, protocol.RouteStatic, protocol.AdminDistStatic)
	if err == nil {
		m.LogEvent(EventRouteChange, routerID, "", "static route added", map[string]interface{}{
			"network": cidr, "next_hop": nextHop,
		})
	}
	return err
}

func errDeviceNotFound(id string) error {
	return &deviceError{id: id}
}

type deviceError struct{ id string }

func (e *deviceError) Error() string { return "device " + e.id + " not found" }

// RefreshRipNeighbors rebuilds RIP neighbor tables from topology links.
func (m *Manager) RefreshRipNeighbors() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, link := range m.Links {
		src, srcOk := m.Routers[link.SourceNodeID]
		dst, dstOk := m.Routers[link.TargetNodeID]
		if !srcOk || !dstOk {
			continue
		}
		srcIP := src.Interfaces[link.SourcePortID]
		dstIP := dst.Interfaces[link.TargetPortID]
		if src.Rip != nil && src.Rip.Enabled && dstIP != "" {
			src.Rip.AddNeighbor(link.SourcePortID, dstIP)
		}
		if dst.Rip != nil && dst.Rip.Enabled && srcIP != "" {
			dst.Rip.AddNeighbor(link.TargetPortID, srcIP)
		}
	}
}

// FindNeighborOnPort returns the remote router and port for a given local port.
func (m *Manager) FindNeighborOnPort(routerID, portID string) (string, string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, link := range m.Links {
		if link.SourceNodeID == routerID && link.SourcePortID == portID {
			return link.TargetNodeID, link.TargetPortID, true
		}
		if link.TargetNodeID == routerID && link.TargetPortID == portID {
			return link.SourceNodeID, link.SourcePortID, true
		}
	}
	return "", "", false
}

// RunOspfHelloCycle exchanges Hello packets between OSPF-enabled neighbors.
func (m *Manager) RunOspfHelloCycle() {
	simTime := m.SimNow()
	m.mu.RLock()
	routers := make([]*Router, 0, len(m.Routers))
	for _, r := range m.Routers {
		routers = append(routers, r)
	}
	m.mu.RUnlock()

	for _, router := range routers {
		if router.Ospf == nil || !router.Ospf.Enabled {
			continue
		}

		for portID, ifaceCfg := range router.Ospf.Interfaces {
			neighborRouterID, neighborPortID, found := m.FindNeighborOnPort(router.ID, portID)
			if !found {
				continue
			}

			neighborRouter, ok := m.GetRouter(neighborRouterID)
			if !ok || neighborRouter.Ospf == nil || !neighborRouter.Ospf.Enabled {
				continue
			}

			hello := router.Ospf.GenerateHello(portID, ifaceCfg.Mask)
			m.SendOspfHelloOnWire(router, portID, hello)

			neighborMask := ""
			if cfg, ok := neighborRouter.Ospf.Interfaces[neighborPortID]; ok {
				neighborMask = cfg.Mask
			}
			replyHello := neighborRouter.Ospf.GenerateHello(neighborPortID, neighborMask)
			m.SendOspfHelloOnWire(neighborRouter, neighborPortID, replyHello)
		}

		router.Ospf.CheckDeadInterval(simTime)
		m.FloodOspfLSAs(router)
		m.RunOspfSPF(router)
	}
}

// FloodOspfLSAs generates and distributes LSAs across the OSPF domain.
func (m *Manager) FloodOspfLSAs(router *Router) {
	if router.Ospf == nil || !router.Ospf.Enabled {
		return
	}

	subnets := router.GetConnectedSubnets()
	lsa := router.Ospf.GenerateRouterLsa(subnets)
	router.Ospf.UpdateLSDB(lsa)

	m.mu.RLock()
	routers := make([]*Router, 0, len(m.Routers))
	for _, r := range m.Routers {
		if r.Ospf != nil && r.Ospf.Enabled {
			routers = append(routers, r)
		}
	}
	m.mu.RUnlock()

	for _, peer := range routers {
		if peer.ID == router.ID {
			continue
		}
		peer.Ospf.UpdateLSDB(lsa)
	}
}

// RunOspfSPF computes shortest paths and installs OSPF routes.
func (m *Manager) RunOspfSPF(router *Router) {
	if router.Ospf == nil || !router.Ospf.Enabled {
		return
	}

	router.RemoveRoutesByProtocol(protocol.RouteOSPF)

	lsdb := router.Ospf.GetLSDB()
	results := router.Ospf.ComputeSPF(lsdb, string(router.Ospf.RouterID))

	for dest, spf := range results {
		if !isSubnetCIDR(dest) {
			continue
		}
		nextHop, iface := router.ResolveSubnetNextHop(spf.NextHop)
		if iface == "" {
			continue
		}
		_ = router.AddRoute(dest, nextHop, iface, spf.Metric, protocol.RouteOSPF, protocol.AdminDistOSPF)
	}
}

// RunRipUpdateCycle sends RIP updates between neighbors.
func (m *Manager) RunRipUpdateCycle() {
	m.mu.RLock()
	routers := make([]*Router, 0, len(m.Routers))
	for _, r := range m.Routers {
		routers = append(routers, r)
	}
	m.mu.RUnlock()

	for _, router := range routers {
		if router.Rip == nil || !router.Rip.Enabled {
			continue
		}

		localRoutes := router.BuildRipAdvertisement()

		for portID := range router.Rip.Neighbors {
			neighborRouterID, neighborPortID, found := m.FindNeighborOnPort(router.ID, portID)
			if !found {
				continue
			}

			neighborRouter, ok := m.GetRouter(neighborRouterID)
			if !ok || neighborRouter.Rip == nil || !neighborRouter.Rip.Enabled {
				continue
			}

			update := router.Rip.BuildUpdate(localRoutes)
			m.SendRipUpdateOnWire(router, portID, update)

			replyUpdate := neighborRouter.Rip.BuildUpdate(neighborRouter.BuildRipAdvertisement())
			m.SendRipUpdateOnWire(neighborRouter, neighborPortID, replyUpdate)
		}
	}
}

// ScheduleProtocolTimers registers recurring OSPF and RIP timer events.
func (m *Manager) ScheduleProtocolTimers(scheduler *engine.Scheduler) {
	scheduler.Schedule(engine.EventTimerOSPF, engine.OspfHelloInterval, nil)
	scheduler.Schedule(engine.EventTimerRIP, engine.RipUpdateInterval, nil)
	m.ScheduleExtendedTimers(scheduler)
}

// HandleTimerEvent dispatches protocol timer events.
func (m *Manager) HandleTimerEvent(eventType engine.EventType, scheduler *engine.Scheduler) {
	switch eventType {
	case engine.EventTimerOSPF:
		m.RunOspfHelloCycle()
		scheduler.Schedule(engine.EventTimerOSPF, engine.OspfHelloInterval, nil)
	case engine.EventTimerRIP:
		m.RunRipUpdateCycle()
		scheduler.Schedule(engine.EventTimerRIP, engine.RipUpdateInterval, nil)
	default:
		m.HandleExtendedTimer(eventType, scheduler)
	}
}

// RouteTableRow is a JSON-serializable routing table entry for the UI.
type RouteTableRow struct {
	Protocol  string `json:"protocol"`
	Network   string `json:"network"`
	Metric    int    `json:"metric"`
	NextHop   string `json:"next_hop"`
	Interface string `json:"interface"`
}

// GetRouteTable returns the formatted routing table for a router.
func (m *Manager) GetRouteTable(routerID string) ([]RouteTableRow, error) {
	router, ok := m.GetRouter(routerID)
	if !ok {
		return nil, errDeviceNotFound(routerID)
	}
	return router.FormatRouteTable(), nil
}

// OspfNeighborRow is a JSON-serializable OSPF neighbor entry.
type OspfNeighborRow struct {
	RouterID  string `json:"router_id"`
	State     string `json:"state"`
	Interface string `json:"interface"`
}

// GetOspfNeighbors returns OSPF neighbor table for a router.
func (m *Manager) GetOspfNeighbors(routerID string) ([]OspfNeighborRow, error) {
	router, ok := m.GetRouter(routerID)
	if !ok {
		return nil, errDeviceNotFound(routerID)
	}
	if router.Ospf == nil {
		return []OspfNeighborRow{}, nil
	}

	neighbors := router.Ospf.GetNeighbors()
	rows := make([]OspfNeighborRow, 0, len(neighbors))
	for _, n := range neighbors {
		rows = append(rows, OspfNeighborRow{
			RouterID:  string(n.RouterID),
			State:     string(n.State),
			Interface: n.Interface,
		})
	}
	return rows, nil
}

func isSubnetCIDR(s string) bool {
	_, _, err := net.ParseCIDR(s)
	return err == nil
}
