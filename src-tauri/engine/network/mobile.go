package network

import (
	"fmt"
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
	// UETable maps assigned UE IPs to their UE node IDs for downlink delivery.
	UETable       map[pdu.IPAddress]string
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

// AttachMobileUE connects a UE to cellular gateway with IP and wires up the radio forwarding path.
func (m *Manager) AttachMobileUE(ueID, gatewayID string, ip pdu.IPAddress) error {
	ue, ok := m.MobileUEs[ueID]
	if !ok {
		return errDeviceNotFound(ueID)
	}
	gw, gwOk := m.GetCellularGateway(gatewayID)
	if !gwOk {
		return errDeviceNotFound(gatewayID)
	}
	ue.Attach(gatewayID, ip)

	// Register the UE's IP in the gateway's ARP-equivalent table so the gateway
	// can forward reply traffic back to the UE without a real radio ARP cycle.
	gw.mu.Lock()
	if gw.UETable == nil {
		gw.UETable = make(map[pdu.IPAddress]string)
	}
	gw.UETable[ip] = ueID
	gw.mu.Unlock()
	return nil
}

// SendIPFromUE builds an IP packet from the UE and sends it through the gateway.
func (m *Manager) SendIPFromUE(ueID string, destIP pdu.IPAddress) error {
	ue, ok := m.MobileUEs[ueID]
	if !ok {
		return errDeviceNotFound(ueID)
	}
	ue.mu.RLock()
	srcIP := ue.IP
	gwID := ue.GatewayID
	registered := ue.Registered
	ue.mu.RUnlock()

	if !registered || gwID == "" {
		return fmt.Errorf("UE %s not attached to a gateway", ueID)
	}

	gw, ok := m.GetCellularGateway(gwID)
	if !ok {
		return errDeviceNotFound(gwID)
	}

	// Build a minimal ICMP echo to carry through the gateway — callers can use
	// StartPing with the UE's assigned IP as a stand-in for a real packet.
	ip := &pdu.IPv4Packet{
		Version: 4, TTL: 64, Protocol: pdu.ProtoICMP,
		SourceIP:      srcIP,
		DestinationIP: destIP,
		ICMP:          pdu.NewEchoRequest(1, 1, []byte("ue-test")),
	}
	m.forwardIPThroughGateway(gw, ip)
	return nil
}

// forwardIPThroughGateway injects an IP packet into the gateway's wired forwarding path.
// This is the uplink direction: UE → Gateway → wired network.
func (m *Manager) forwardIPThroughGateway(gw *CellularGateway, ip *pdu.IPv4Packet) {
	gw.mu.RLock()
	// Find the first non-cellular (wired) interface to forward out of.
	outPort := ""
	var outMAC pdu.MACAddress
	for portID, mac := range gw.InterfaceMAC {
		if portID != "Cellular0/0/0" {
			outPort = portID
			outMAC = mac
			break
		}
	}
	gw.mu.RUnlock()

	if outPort == "" {
		return
	}

	simTime := m.SimNow()
	_ = simTime

	// Use broadcast MAC as dst — ARP will resolve on the first hop.
	frame, err := pdu.NewIPv4Frame(pdu.MACBroadcast, outMAC, ip)
	if err != nil {
		return
	}
	wire := m.wrapWireFrame(gw.ID, outPort, frame)
	m.forwardFromDevice(gw.ID, outPort, wire)
}

// deliverToUE handles a frame destined for a registered MobileUE.
// This is the downlink direction: wired network → Gateway → UE.
func (m *Manager) deliverToUE(ueID string, wire *pdu.WireFrame) {
	ue, ok := m.MobileUEs[ueID]
	if !ok {
		return
	}
	ue.mu.RLock()
	registered := ue.Registered
	ue.mu.RUnlock()
	if !registered {
		return
	}

	payload, err := pdu.DecodeFramePayload(wire.Frame)
	if err != nil || payload == nil || payload.IP == nil {
		return
	}
	ip := payload.IP
	if ip.Protocol == pdu.ProtoICMP && ip.ICMP != nil {
		if ip.ICMP.Type == pdu.ICMPEchoReply {
			m.completePing(ueID, ip.SourceIP, ip.ICMP.ID, ip.ICMP.Sequence, m.SimNow())
		}
	}
}

// GetMobileUE returns a UE by ID.
func (m *Manager) GetMobileUE(id string) (*MobileUE, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ue, ok := m.MobileUEs[id]
	return ue, ok
}
