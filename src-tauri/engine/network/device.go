package network

// DeviceType identifies a node in the simulated topology.
type DeviceType string

const (
	DeviceRouter   DeviceType = "ROUTER"
	DeviceSwitch   DeviceType = "SWITCH"
	DeviceHost     DeviceType = "HOST"
	DeviceAP       DeviceType = "ACCESS_POINT"
	DeviceASA      DeviceType = "ASA"
	DeviceVoIP     DeviceType = "VOIP_PHONE"
	DeviceCellular DeviceType = "CELLULAR_GATEWAY"
	DeviceMobile   DeviceType = "MOBILE_UE"
)

// NodeTypeOf returns the device type for a node ID.
func (m *Manager) NodeTypeOf(nodeID string) (DeviceType, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.Routers[nodeID]; ok {
		return DeviceRouter, true
	}
	if _, ok := m.Switches[nodeID]; ok {
		return DeviceSwitch, true
	}
	if _, ok := m.Hosts[nodeID]; ok {
		return DeviceHost, true
	}
	if _, ok := m.AccessPoints[nodeID]; ok {
		return DeviceAP, true
	}
	if _, ok := m.ASAFirewalls[nodeID]; ok {
		return DeviceASA, true
	}
	return "", false
}
