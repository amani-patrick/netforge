package network

import (
	"fmt"

	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// RemoveDevice deletes a node and all its secondary state from the simulation:
// – removes the device from all type maps
// – prunes every topology link touching it
// – removes OSPF adjacencies on neighbouring routers that referenced this device
// – clears ARP/IKE entries pointing at this device's IPs on every other router
// – removes routing table entries learned through this device (OSPF/EIGRP/BGP/RIP)
// – cancels any in-flight ping sessions sourced from or targeting this device
func (m *Manager) RemoveDevice(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Collect all IP addresses owned by the device being removed so we can
	// purge references on neighbouring routers.
	removedIPs := m.collectDeviceIPsLocked(nodeID)

	found := false
	if _, ok := m.Routers[nodeID]; ok {
		delete(m.Routers, nodeID)
		found = true
	}
	if _, ok := m.Switches[nodeID]; ok {
		delete(m.Switches, nodeID)
		found = true
	}
	if _, ok := m.Hosts[nodeID]; ok {
		delete(m.Hosts, nodeID)
		found = true
	}
	if _, ok := m.AccessPoints[nodeID]; ok {
		delete(m.AccessPoints, nodeID)
		found = true
	}
	if _, ok := m.ASAFirewalls[nodeID]; ok {
		delete(m.ASAFirewalls, nodeID)
		found = true
	}
	if _, ok := m.VoIPPhones[nodeID]; ok {
		delete(m.VoIPPhones, nodeID)
		found = true
	}
	if _, ok := m.CellularGateways[nodeID]; ok {
		delete(m.CellularGateways, nodeID)
		found = true
	}
	if _, ok := m.MobileUEs[nodeID]; ok {
		delete(m.MobileUEs, nodeID)
		found = true
	}
	if !found {
		return fmt.Errorf("device %s not found", nodeID)
	}

	// Remove all links connected to this device.
	filtered := make([]TopologyLink, 0, len(m.Links))
	for _, l := range m.Links {
		if l.SourceNodeID != nodeID && l.TargetNodeID != nodeID {
			filtered = append(filtered, l)
		}
	}
	m.Links = filtered

	// Secondary cleanup on every remaining router.
	for _, r := range m.Routers {
		m.purgeRouterReferencesLocked(r, nodeID, removedIPs)
	}

	// Cancel in-flight ping sessions touching this device.
	for id, sess := range m.pingSessions {
		if sess.SourceID == nodeID {
			m.pendingPingResults = append(m.pendingPingResults, PingResult{
				SourceID: sess.SourceID, DestIP: sess.DestIP,
				Success: false, Message: "source device removed",
				RequestID: sess.ReplyConn.RequestID,
			})
			delete(m.pingSessions, id)
		}
	}

	m.LogEvent(EventProtocol, nodeID, "", "device removed", nil)
	return nil
}

// collectDeviceIPsLocked returns all IP addresses assigned to a device (called with m.mu held).
func (m *Manager) collectDeviceIPsLocked(nodeID string) []pdu.IPAddress {
	var ips []pdu.IPAddress
	if r, ok := m.Routers[nodeID]; ok {
		r.mu.RLock()
		for _, ip := range r.Interfaces {
			ips = append(ips, ip)
		}
		r.mu.RUnlock()
	}
	if h, ok := m.Hosts[nodeID]; ok {
		ips = append(ips, h.IP)
	}
	if gw, ok := m.CellularGateways[nodeID]; ok {
		gw.mu.RLock()
		for _, ip := range gw.Interfaces {
			ips = append(ips, ip)
		}
		if gw.PublicIP != "" {
			ips = append(ips, gw.PublicIP)
		}
		gw.mu.RUnlock()
	}
	if ue, ok := m.MobileUEs[nodeID]; ok {
		ue.mu.RLock()
		if ue.IP != "" {
			ips = append(ips, ue.IP)
		}
		ue.mu.RUnlock()
	}
	return ips
}

// purgeRouterReferencesLocked cleans up all traces of removedDevice on router r.
// Called with m.mu held; acquires r.mu internally.
func (m *Manager) purgeRouterReferencesLocked(r *Router, removedNodeID string, removedIPs []pdu.IPAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ipSet := make(map[pdu.IPAddress]bool, len(removedIPs))
	for _, ip := range removedIPs {
		ipSet[ip] = true
	}

	// 1. Remove ARP cache entries for the removed device's IPs.
	for ip := range r.ArpCache {
		if ipSet[ip] {
			delete(r.ArpCache, ip)
		}
	}

	// 2. Drain queued packets destined for the removed device's IPs.
	for ip := range r.PacketQueue {
		if ipSet[ip] {
			delete(r.PacketQueue, ip)
		}
	}

	// 3. Remove CDP neighbor entries learned from the removed device.
	live := r.CDPNeighbors[:0]
	for _, n := range r.CDPNeighbors {
		if n.DeviceID != removedNodeID {
			live = append(live, n)
		}
	}
	r.CDPNeighbors = live

	// 4. Remove port-neighbor mappings that pointed at the removed device.
	for port, pn := range r.PortNeighbors {
		if pn.RouterID == removedNodeID {
			delete(r.PortNeighbors, port)
		}
	}

	// 5. Remove OSPF adjacency for the removed device and purge its LSDB entries.
	if r.Ospf != nil && r.Ospf.Enabled {
		// Neighbors keyed by IP — remove any neighbor whose IP belongs to the removed device.
		for neighborIP := range r.Ospf.Neighbors {
			if ipSet[neighborIP] {
				delete(r.Ospf.Neighbors, neighborIP)
			}
		}
		// LSDB keyed by router-ID string — remove the departed router's LSA.
		delete(r.Ospf.LSDB, removedNodeID)
		for _, ip := range removedIPs {
			delete(r.Ospf.LSDB, string(ip))
		}
	}

	// 6. Remove RIP neighbor entries pointing at the removed device's IPs.
	if r.Rip != nil {
		for port, neighborIP := range r.Rip.Neighbors {
			if ipSet[neighborIP] {
				delete(r.Rip.Neighbors, port)
			}
		}
	}

	// 7. Remove routes that were installed via the removed device (next-hop = its IP,
	//    or the route was learned via an interface that led directly to that device).
	surviving := r.RoutingTable[:0]
	for _, route := range r.RoutingTable {
		// Always keep connected and static routes unless next-hop is the removed device.
		if route.Protocol == protocol.RouteConnected {
			surviving = append(surviving, route)
			continue
		}
		if ipSet[route.NextHop] {
			continue // drop: next-hop belongs to removed device
		}
		surviving = append(surviving, route)
	}
	r.RoutingTable = surviving

	// 8. Remove IKE peer state for the removed device's IPs.
	for ip := range r.IKEPeers {
		if ipSet[pdu.IPAddress(ip)] {
			delete(r.IKEPeers, ip)
		}
	}
}

// RemoveLink deletes a single topology link by ID.
func (m *Manager) RemoveLink(linkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	filtered := make([]TopologyLink, 0, len(m.Links))
	for _, l := range m.Links {
		if l.ID == linkID {
			found = true
			continue
		}
		filtered = append(filtered, l)
	}
	if !found {
		return fmt.Errorf("link %s not found", linkID)
	}
	m.Links = filtered
	m.LogEvent(EventProtocol, "", "", "link removed: "+linkID, nil)
	return nil
}
