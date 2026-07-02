package network

import (
	"encoding/json"
)

// TopologySnapshot is the serializable full simulation state.
type TopologySnapshot struct {
	Routers  []RouterSnapshot  `json:"routers"`
	Switches []SwitchSnapshot  `json:"switches"`
	Hosts    []HostSnapshot    `json:"hosts"`
	Links    []TopologyLink    `json:"links"`
}

// RouterSnapshot is a JSON-serializable router state.
type RouterSnapshot struct {
	ID            string              `json:"id"`
	Interfaces    []IfaceSnapshot     `json:"interfaces"`
	StaticRoutes  []StaticRouteSnap   `json:"static_routes,omitempty"`
	OspfEnabled   bool                `json:"ospf_enabled,omitempty"`
	OspfProcessID int                 `json:"ospf_process_id,omitempty"`
	OspfNetworks  []OspfNetworkSnap   `json:"ospf_networks,omitempty"`
	RipEnabled    bool                `json:"rip_enabled,omitempty"`
	RipNetworks   []string            `json:"rip_networks,omitempty"`
	EigrpEnabled  bool                `json:"eigrp_enabled,omitempty"`
	EigrpAS       int                 `json:"eigrp_as,omitempty"`
	EigrpNetworks []string            `json:"eigrp_networks,omitempty"`
	BgpEnabled    bool                `json:"bgp_enabled,omitempty"`
	BgpLocalAS    int                 `json:"bgp_local_as,omitempty"`
}

// IfaceSnapshot is a router interface.
type IfaceSnapshot struct {
	PortID string `json:"port_id"`
	IP     string `json:"ip"`
	Mask   string `json:"mask"`
	MAC    string `json:"mac"`
}

// StaticRouteSnap is a static route entry.
type StaticRouteSnap struct {
	CIDR      string `json:"cidr"`
	NextHop   string `json:"next_hop"`
	Interface string `json:"interface"`
	Metric    int    `json:"metric"`
}

// OspfNetworkSnap is an OSPF network statement.
type OspfNetworkSnap struct {
	CIDR string `json:"cidr"`
	Area int    `json:"area"`
}

// SwitchSnapshot is a JSON-serializable switch state.
type SwitchSnapshot struct {
	ID    string   `json:"id"`
	Ports []string `json:"ports"`
	VLANs []int    `json:"vlans,omitempty"`
}

// ExportSnapshot captures the full topology for persistence.
func (m *Manager) ExportSnapshot() TopologySnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := TopologySnapshot{
		Links: make([]TopologyLink, len(m.Links)),
	}
	copy(snap.Links, m.Links)

	for _, r := range m.Routers {
		snap.Routers = append(snap.Routers, r.Snapshot())
	}
	for _, sw := range m.Switches {
		snap.Switches = append(snap.Switches, sw.Snapshot())
	}
	for _, h := range m.Hosts {
		snap.Hosts = append(snap.Hosts, h.Snapshot())
	}
	return snap
}

// ImportSnapshot replaces the manager state from a snapshot.
func (m *Manager) ImportSnapshot(snap TopologySnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Routers = make(map[string]*Router)
	m.Switches = make(map[string]*Switch)
	m.Hosts = make(map[string]*Host)
	m.Links = snap.Links

	for _, rs := range snap.Routers {
		r := RestoreRouter(rs)
		m.Routers[r.ID] = r
	}
	for _, ss := range snap.Switches {
		sw := RestoreSwitch(ss)
		m.Switches[sw.ID] = sw
	}
	for _, hs := range snap.Hosts {
		h := RestoreHost(hs)
		m.Hosts[h.ID] = h
	}

	for _, link := range m.Links {
		m.applyLinkState(link)
	}
}

// SnapshotJSON returns the topology as JSON bytes.
func (m *Manager) SnapshotJSON() ([]byte, error) {
	return json.MarshalIndent(m.ExportSnapshot(), "", "  ")
}

// LoadSnapshotJSON restores from JSON bytes.
func (m *Manager) LoadSnapshotJSON(data []byte) error {
	var snap TopologySnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	m.ImportSnapshot(snap)
	return nil
}
