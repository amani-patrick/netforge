package network

import (
	"sync"

	"netforge/engine/pdu"
)

// VoIPPhone is an IP telephony endpoint.
type VoIPPhone struct {
	ID            string
	MAC           pdu.MACAddress
	Extension     string
	VoiceVLAN     pdu.VLANID
	DataIP        pdu.IPAddress
	VoiceIP       pdu.IPAddress
	CallManagerIP pdu.IPAddress
	PortID        string
	UplinkNode    string
	Registered    bool
	mu            sync.RWMutex
}

// NewVoIPPhone creates a Cisco-style IP phone.
func NewVoIPPhone(id string) *VoIPPhone {
	return &VoIPPhone{
		ID: id, PortID: "Fa0", Extension: "1001",
		VoiceVLAN: pdu.VLANID(150),
	}
}

// Configure sets phone parameters after DHCP/SCCP registration.
func (p *VoIPPhone) Configure(ext string, voiceIP, dataIP, cm pdu.IPAddress, vlan pdu.VLANID) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Extension = ext
	p.VoiceIP = voiceIP
	p.DataIP = dataIP
	p.CallManagerIP = cm
	p.VoiceVLAN = vlan
	p.Registered = true
}

// SetUplink records switch/AP connection.
func (p *VoIPPhone) SetUplink(nodeID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.UplinkNode = nodeID
}

// CallManager is a CUCM / VoIP server.
type CallManager struct {
	ID        string
	IP        pdu.IPAddress
	Phones    map[string]*VoIPPhone
	DHCPPool  string
	mu        sync.RWMutex
}

// NewCallManager creates a call manager appliance.
func NewCallManager(id string) *CallManager {
	return &CallManager{ID: id, Phones: make(map[string]*VoIPPhone)}
}

// RegisterPhone adds a phone to the CM database.
func (cm *CallManager) RegisterPhone(phone *VoIPPhone) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.Phones[phone.ID] = phone
	phone.Registered = true
}

// AddVoIPPhone provisions a phone in the topology.
func (m *Manager) AddVoIPPhone(id string) *VoIPPhone {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.VoIPPhones == nil {
		m.VoIPPhones = make(map[string]*VoIPPhone)
	}
	phone := NewVoIPPhone(id)
	m.VoIPPhones[id] = phone
	return phone
}

// GetVoIPPhone returns a phone by ID.
func (m *Manager) GetVoIPPhone(id string) (*VoIPPhone, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.VoIPPhones[id]
	return p, ok
}

// AddCallManager provisions a VoIP call manager.
func (m *Manager) AddCallManager(id string) *CallManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.CallManagers == nil {
		m.CallManagers = make(map[string]*CallManager)
	}
	cm := NewCallManager(id)
	m.CallManagers[id] = cm
	return cm
}

// ConfigureVoIPDHCP sets DHCP pool with option 150 (Call Manager) for phones.
func (r *Router) ConfigureVoIPDHCPPool(name, network string, routerGW, cmIP pdu.IPAddress, voiceVLAN int) {
	pool := DHCPPool{
		Name: name, Network: network,
		DefaultRouter: routerGW, DNSServer: cmIP,
		VoIPOption150: cmIP, VoiceVLAN: pdu.VLANID(voiceVLAN),
	}
	r.AddDHCPPool(pool)
}
