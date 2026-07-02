package network

import (
	"net"

	"netforge/engine"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// ConfigureSwitchVLAN sets access VLAN on a switch port.
func (m *Manager) ConfigureSwitchVLAN(switchID, portID string, vlan int) error {
	sw, ok := m.GetSwitch(switchID)
	if !ok {
		return errDeviceNotFound(switchID)
	}
	sw.CreateVLAN(pdu.VLANID(vlan))
	sw.SetPortAccessVLAN(portID, pdu.VLANID(vlan))
	return nil
}

// ConfigureSwitchTrunk sets trunk mode on a switch port.
func (m *Manager) ConfigureSwitchTrunk(switchID, portID string, native int, allowed []int) error {
	sw, ok := m.GetSwitch(switchID)
	if !ok {
		return errDeviceNotFound(switchID)
	}
	allowedVLANs := make([]pdu.VLANID, len(allowed))
	for i, v := range allowed {
		allowedVLANs[i] = pdu.VLANID(v)
		sw.CreateVLAN(pdu.VLANID(v))
	}
	sw.SetPortTrunk(portID, pdu.VLANID(native), allowedVLANs)
	return nil
}

// ConfigureRouterSubInterface creates inter-VLAN routing subinterface.
func (m *Manager) ConfigureRouterSubInterface(routerID, parentPort string, vlan int, ip, mask string) (string, error) {
	router, ok := m.GetRouter(routerID)
	if !ok {
		return "", errDeviceNotFound(routerID)
	}
	subID := router.AddSubInterface(parentPort, vlan, pdu.IPAddress(ip), mask)
	return subID, nil
}

// RunSTPOnAllSwitches runs STP election across all switches.
func (m *Manager) RunSTPOnAllSwitches() {
	m.mu.RLock()
	switches := make([]*Switch, 0, len(m.Switches))
	for _, sw := range m.Switches {
		switches = append(switches, sw)
	}
	m.mu.RUnlock()
	for _, sw := range switches {
		sw.RunSTP(switches)
	}
}

// RunEigrpUpdateCycle exchanges EIGRP updates between neighbors.
func (m *Manager) RunEigrpUpdateCycle() {
	m.mu.RLock()
	routers := make([]*Router, 0, len(m.Routers))
	for _, r := range m.Routers {
		routers = append(routers, r)
	}
	m.mu.RUnlock()

	for _, router := range routers {
		if router.Eigrp == nil || !router.Eigrp.Enabled {
			continue
		}
		local := router.BuildEigrpAdvertisement()
		advert := router.Eigrp.GetAdvertisement(local)

		for portID := range router.Interfaces {
			neighborRouterID, neighborPortID, found := m.FindNeighborOnPort(router.ID, portID)
			if !found {
				continue
			}
			neighborRouter, ok := m.GetRouter(neighborRouterID)
			if !ok || neighborRouter.Eigrp == nil || !neighborRouter.Eigrp.Enabled {
				continue
			}
			senderIP := router.Interfaces[portID]
			newRoutes := neighborRouter.Eigrp.ProcessUpdate(advert, senderIP, neighborPortID, m.SimNow())
			for _, route := range newRoutes {
				_ = neighborRouter.AddRoute(route.Network, route.NextHop, route.Interface, route.Metric, protocol.RouteEIGRP, protocol.AdminDistEIGRP)
			}
		}
	}
}

// RunBgpUpdateCycle exchanges BGP updates between peers.
func (m *Manager) RunBgpUpdateCycle() {
	m.mu.RLock()
	routers := make([]*Router, 0, len(m.Routers))
	for _, r := range m.Routers {
		routers = append(routers, r)
	}
	m.mu.RUnlock()

	for _, router := range routers {
		if router.Bgp == nil || !router.Bgp.Enabled {
			continue
		}
		local := router.BuildBgpAdvertisement()
		advert := router.Bgp.GetAdvertisement(local)

		for peerIP := range router.Bgp.Peers {
			neighborRouterID, found := m.findRouterByIP(peerIP)
			if !found {
				continue
			}
			neighborRouter, ok := m.GetRouter(neighborRouterID)
			if !ok || neighborRouter.Bgp == nil || !neighborRouter.Bgp.Enabled {
				continue
			}
			newRoutes := neighborRouter.Bgp.ProcessUpdate(advert, peerIP)
			for _, route := range newRoutes {
				_ = neighborRouter.AddRoute(route.Prefix, route.NextHop, "", len(route.ASPath), protocol.RouteBGP, protocol.AdminDistBGP)
			}
		}
	}
}

func (m *Manager) findRouterByIP(ip pdu.IPAddress) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, r := range m.Routers {
		for _, ifaceIP := range r.Interfaces {
			if ifaceIP == ip {
				return id, true
			}
		}
	}
	return "", false
}

// RunCDPCycle sends CDP advertisements on all router ports.
func (m *Manager) RunCDPCycle() {
	m.mu.RLock()
	routers := make([]*Router, 0, len(m.Routers))
	for _, r := range m.Routers {
		routers = append(routers, r)
	}
	m.mu.RUnlock()

	for _, router := range routers {
		for portID := range router.Interfaces {
			if !router.IsInterfaceUp(portID) {
				continue
			}
			cdp := router.BuildCDPAdvertisement(portID)
			frame := &pdu.EthernetFrame{DestinationMAC: pdu.MACBroadcast, SourceMAC: router.InterfaceMAC[portID]}
			_ = pdu.EncodeFramePayload(frame, &pdu.FramePayload{Type: pdu.PayloadCDP, CDP: cdp})
			m.forwardFromDevice(router.ID, portID, &pdu.WireFrame{Frame: frame})
		}
	}
}

// HostDHCPRequest triggers DHCP on a host.
func (m *Manager) HostDHCPRequest(hostID string) (pdu.IPAddress, pdu.IPAddress, pdu.IPAddress, string, error) {
	host, ok := m.GetHost(hostID)
	if !ok {
		return "", "", "", "", errDeviceNotFound(hostID)
	}

	uplinkNode := host.UplinkNode
	if uplinkNode == "" {
		return "", "", "", "", errDeviceNotFound("no uplink")
	}

	router, ok := m.GetRouter(uplinkNode)
	if !ok {
		// try through switch to find router - simplified: scan routers
		m.mu.RLock()
		for _, r := range m.Routers {
			router = r
			ok = true
			break
		}
		m.mu.RUnlock()
	}
	if !ok {
		return "", "", "", "", errDeviceNotFound("no dhcp server")
	}

	discover := &pdu.DHCPPacket{
		Op: 1, MessageType: pdu.DHCPDiscover, XID: 0x1234,
		ClientMAC: host.MAC, ClientIP: "0.0.0.0",
	}
	inPort := ""
	for portID, ip := range router.Interfaces {
		_, ipNet, err := net.ParseCIDR("192.168.0.0/16")
		if err == nil && ipNet.Contains(net.ParseIP(string(ip))) {
			inPort = portID
			break
		}
		_ = portID
	}
	if inPort == "" {
		for p := range router.Interfaces {
			inPort = p
			break
		}
	}

	ack := router.HandleDHCPRequest(discover, inPort)
	if ack == nil {
		return "", "", "", "", errDeviceNotFound("dhcp pool not configured")
	}

	host.Configure(ack.YourIP, ack.SubnetMask, ack.GatewayIP, host.MAC)
	return ack.YourIP, ack.GatewayIP, ack.DNSServer, ack.SubnetMask, nil
}

// ScheduleExtendedTimers registers Tier 2/3 protocol timers.
func (m *Manager) ScheduleExtendedTimers(scheduler *engine.Scheduler) {
	scheduler.Schedule(engine.EventTimerEIGRP, engine.EigrpUpdateInterval, nil)
	scheduler.Schedule(engine.EventTimerBGP, engine.BgpUpdateInterval, nil)
	scheduler.Schedule(engine.EventTimerCDP, engine.CdpInterval, nil)
	scheduler.Schedule(engine.EventTimerSTP, engine.StpInterval, nil)
	scheduler.Schedule(engine.EventTimerHSRP, engine.HsrpHelloInterval, nil)
	scheduler.Schedule(engine.EventTimerVTP, engine.VtpAdvertInterval, nil)
}

// HandleExtendedTimer dispatches Tier 2/3 timer events.
func (m *Manager) HandleExtendedTimer(eventType engine.EventType, scheduler *engine.Scheduler) {
	switch eventType {
	case engine.EventTimerEIGRP:
		m.RunEigrpUpdateCycle()
		scheduler.Schedule(engine.EventTimerEIGRP, engine.EigrpUpdateInterval, nil)
	case engine.EventTimerBGP:
		m.RunBgpUpdateCycle()
		scheduler.Schedule(engine.EventTimerBGP, engine.BgpUpdateInterval, nil)
	case engine.EventTimerCDP:
		m.RunCDPCycle()
		scheduler.Schedule(engine.EventTimerCDP, engine.CdpInterval, nil)
	case engine.EventTimerSTP:
		m.RunSTPOnAllSwitches()
		scheduler.Schedule(engine.EventTimerSTP, engine.StpInterval, nil)
	case engine.EventTimerHSRP:
		m.RunHSRPElection()
		scheduler.Schedule(engine.EventTimerHSRP, engine.HsrpHelloInterval, nil)
	case engine.EventTimerVTP:
		m.PropagateVTP()
		scheduler.Schedule(engine.EventTimerVTP, engine.VtpAdvertInterval, nil)
	}
}
