package network

import (
	"sync"

	"netforge/engine/pdu"
)

// CellularGateway is an LTE/5G WAN router.
type CellularGateway struct {
	ID            string
	Hostname      string
	Carrier       string
	APN           string
	PublicIP      pdu.IPAddress
	PrivateIP     pdu.IPAddress
	RAT           RAType
	NR            *NRBandConfig
	FiveGC        *FiveGCoreConfig
	Interfaces    map[string]pdu.IPAddress
	InterfaceMAC  map[string]pdu.MACAddress
	InterfaceMask map[string]string
	LTEBand       string
	SignalDBm     int
	Connected     bool
	mu            sync.RWMutex
}

// NewCellularGateway creates an LTE gateway.
func NewCellularGateway(id string) *CellularGateway {
	return &CellularGateway{
		ID: id, Carrier: "LTE", APN: "internet", RAT: RALTE,
		Interfaces: make(map[string]pdu.IPAddress),
		InterfaceMAC: make(map[string]pdu.MACAddress),
		InterfaceMask: make(map[string]string),
		SignalDBm: -75, Connected: true,
	}
}

// AddInterface configures LAN/WAN/cellular interfaces.
func (g *CellularGateway) AddInterface(portID string, ip pdu.IPAddress, mask string, mac pdu.MACAddress) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Interfaces[portID] = ip
	g.InterfaceMask[portID] = mask
	g.InterfaceMAC[portID] = mac
	if portID == "Cellular0/0/0" {
		g.PublicIP = ip
	}
}

// ConnectLTE simulates LTE attach.
func (g *CellularGateway) ConnectLTE(carrier, apn string, publicIP pdu.IPAddress) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Carrier = carrier
	g.APN = apn
	g.PublicIP = publicIP
	g.Connected = true
}

// MobileUE is smartphone / user equipment (LTE or 5G NR).
type MobileUE struct {
	ID           string
	IMSI         string
	IMEI         string
	IP           pdu.IPAddress
	MAC          pdu.MACAddress
	GatewayID    string
	Technology   string
	RAT          RAType
	NRBand       string
	NRARFCN      int
	PDUSessionID int
	Registered   bool
	PortID       string
	mu           sync.RWMutex
}

// NewMobileUE creates a mobile handset.
func NewMobileUE(id string) *MobileUE {
	return &MobileUE{
		ID: id, PortID: "Cellular0",
		Technology: "LTE",
		IMSI: "001010000000001",
	}
}

// Attach registers UE to a cellular gateway.
func (ue *MobileUE) Attach(gatewayID string, ip pdu.IPAddress) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	ue.GatewayID = gatewayID
	ue.IP = ip
}

// AddCellularGateway provisions LTE router.
func (m *Manager) AddCellularGateway(id string) *CellularGateway {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CellularGateways == nil {
		m.CellularGateways = make(map[string]*CellularGateway)
	}
	gw := NewCellularGateway(id)
	m.CellularGateways[id] = gw
	return gw
}

// AddMobileUE provisions a mobile handset.
func (m *Manager) AddMobileUE(id string) *MobileUE {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.MobileUEs == nil {
		m.MobileUEs = make(map[string]*MobileUE)
	}
	ue := NewMobileUE(id)
	m.MobileUEs[id] = ue
	return ue
}

// GetCellularGateway returns LTE gateway by ID.
func (m *Manager) GetCellularGateway(id string) (*CellularGateway, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.CellularGateways[id]
	return g, ok
}

// AttachMobileUE connects a UE to cellular gateway with IP.
func (m *Manager) AttachMobileUE(ueID, gatewayID string, ip pdu.IPAddress) error {
	ue, ok := m.MobileUEs[ueID]
	if !ok {
		return errDeviceNotFound(ueID)
	}
	if _, ok := m.GetCellularGateway(gatewayID); !ok {
		return errDeviceNotFound(gatewayID)
	}
	ue.Attach(gatewayID, ip)
	return nil
}
