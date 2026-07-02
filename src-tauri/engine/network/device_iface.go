package network

import (
	"time"

	"netforge/engine/pdu"
)

// SimDevice is the polymorphic interface for all simulated nodes.
type SimDevice interface {
	DeviceID() string
	DeviceKind() DeviceType
	PortIDs() []string
	Tick(simTime time.Duration)
}

// FrameReceiver handles inbound L2 frames on a port.
type FrameReceiver interface {
	ReceiveFrame(portID string, wire *pdu.WireFrame, simTime time.Duration) []*PortFrame
}

// PortFrame is an outbound frame on a specific port.
type PortFrame struct {
	PortID string
	Wire   *pdu.WireFrame
}

// routerDevice adapts Router to SimDevice.
type routerDevice struct{ r *Router }

func (d routerDevice) DeviceID() string               { return d.r.ID }
func (d routerDevice) DeviceKind() DeviceType         { return DeviceRouter }
func (d routerDevice) PortIDs() []string              { return d.r.PortIDs() }
func (d routerDevice) Tick(_ time.Duration)           {}

// switchDevice adapts Switch to SimDevice.
type switchDevice struct{ s *Switch }

func (d switchDevice) DeviceID() string       { return d.s.ID }
func (d switchDevice) DeviceKind() DeviceType { return DeviceSwitch }
func (d switchDevice) PortIDs() []string      { return d.s.PortIDs() }
func (d switchDevice) Tick(_ time.Duration)   {}

// hostDevice adapts Host to SimDevice.
type hostDevice struct{ h *Host }

func (d hostDevice) DeviceID() string       { return d.h.ID }
func (d hostDevice) DeviceKind() DeviceType { return DeviceHost }
func (d hostDevice) PortIDs() []string      { return []string{d.h.PortID} }
func (d hostDevice) Tick(_ time.Duration)   {}

// apDevice adapts AccessPoint to SimDevice.
type apDevice struct{ ap *AccessPoint }

func (d apDevice) DeviceID() string       { return d.ap.ID }
func (d apDevice) DeviceKind() DeviceType { return DeviceAP }
func (d apDevice) PortIDs() []string {
	return []string{"Dot11Radio0", "GigabitEthernet0"}
}
func (d apDevice) Tick(_ time.Duration) {}

// asaDevice adapts ASAFirewall to SimDevice.
type asaDevice struct{ a *ASAFirewall }

func (d asaDevice) DeviceID() string       { return d.a.ID }
func (d asaDevice) DeviceKind() DeviceType { return DeviceASA }
func (d asaDevice) PortIDs() []string      { return d.a.PortIDs() }
func (d asaDevice) Tick(_ time.Duration)   {}

type voipDevice struct{ p *VoIPPhone }

func (d voipDevice) DeviceID() string       { return d.p.ID }
func (d voipDevice) DeviceKind() DeviceType { return DeviceVoIP }
func (d voipDevice) PortIDs() []string      { return []string{d.p.PortID} }
func (d voipDevice) Tick(_ time.Duration)   {}

type cellularDevice struct{ g *CellularGateway }

func (d cellularDevice) DeviceID() string       { return d.g.ID }
func (d cellularDevice) DeviceKind() DeviceType { return DeviceCellular }
func (d cellularDevice) PortIDs() []string {
	return []string{"GigabitEthernet0/0", "Cellular0/0/0"}
}
func (d cellularDevice) Tick(_ time.Duration) {}

type mobileDevice struct{ ue *MobileUE }

func (d mobileDevice) DeviceID() string       { return d.ue.ID }
func (d mobileDevice) DeviceKind() DeviceType { return DeviceMobile }
func (d mobileDevice) PortIDs() []string      { return []string{d.ue.PortID} }
func (d mobileDevice) Tick(_ time.Duration)   {}

// ListDevices returns all provisioned devices in the topology.
func (m *Manager) ListDevices() []SimDevice {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]SimDevice, 0)
	for _, r := range m.Routers {
		devices = append(devices, routerDevice{r})
	}
	for _, s := range m.Switches {
		devices = append(devices, switchDevice{s})
	}
	for _, h := range m.Hosts {
		devices = append(devices, hostDevice{h})
	}
	for _, ap := range m.AccessPoints {
		devices = append(devices, apDevice{ap})
	}
	for _, a := range m.ASAFirewalls {
		devices = append(devices, asaDevice{a})
	}
	for _, p := range m.VoIPPhones {
		devices = append(devices, voipDevice{p})
	}
	for _, g := range m.CellularGateways {
		devices = append(devices, cellularDevice{g})
	}
	for _, ue := range m.MobileUEs {
		devices = append(devices, mobileDevice{ue})
	}
	return devices
}

// GetDevice returns a device by ID.
func (m *Manager) GetDevice(id string) (SimDevice, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if r, ok := m.Routers[id]; ok {
		return routerDevice{r}, true
	}
	if s, ok := m.Switches[id]; ok {
		return switchDevice{s}, true
	}
	if h, ok := m.Hosts[id]; ok {
		return hostDevice{h}, true
	}
	if ap, ok := m.AccessPoints[id]; ok {
		return apDevice{ap}, true
	}
	if a, ok := m.ASAFirewalls[id]; ok {
		return asaDevice{a}, true
	}
	return nil, false
}

// TickAllDevices advances per-device simulation hooks.
func (m *Manager) TickAllDevices(simTime time.Duration) {
	for _, dev := range m.ListDevices() {
		dev.Tick(simTime)
	}
}
