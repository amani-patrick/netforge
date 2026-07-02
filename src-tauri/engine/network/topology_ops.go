package network

import "fmt"

// RemoveDevice deletes a node and all links touching it from the simulation.
func (m *Manager) RemoveDevice(nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	filtered := make([]TopologyLink, 0, len(m.Links))
	for _, l := range m.Links {
		if l.SourceNodeID != nodeID && l.TargetNodeID != nodeID {
			filtered = append(filtered, l)
		}
	}
	m.Links = filtered
	m.LogEvent(EventProtocol, nodeID, "", "device removed", nil)
	return nil
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
