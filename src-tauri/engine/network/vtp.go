package network

import (
	"fmt"

	"netforge/engine/pdu"
)

// VTPMode is server, client, or transparent.
type VTPMode string

const (
	VTPNone        VTPMode = "off"
	VTPServer      VTPMode = "server"
	VTPClient      VTPMode = "client"
	VTPTransparent VTPMode = "transparent"
)

// VTPConfig holds VLAN Trunking Protocol settings.
type VTPConfig struct {
	Domain   string
	Mode     VTPMode
	Revision int
	Password string
}

// VLANEntry is a named VLAN in the database.
type VLANEntry struct {
	ID   pdu.VLANID
	Name string
}

// ConfigureVTP sets VTP domain and mode on a switch.
func (sw *Switch) ConfigureVTP(domain string, mode VTPMode) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.VTP = VTPConfig{Domain: domain, Mode: mode, Revision: sw.VTP.Revision}
}

// SetVLANName assigns a name to a VLAN.
func (sw *Switch) SetVLANName(vlan pdu.VLANID, name string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.VLANs[vlan] = true
	if sw.VLANNames == nil {
		sw.VLANNames = make(map[pdu.VLANID]string)
	}
	sw.VLANNames[vlan] = name
	if sw.VTP.Mode == VTPServer {
		sw.VTP.Revision++
	}
}

// PropagateVTP syncs VLANs from server switches to clients in the same domain.
func (m *Manager) PropagateVTP() {
	m.mu.RLock()
	switches := make([]*Switch, 0, len(m.Switches))
	for _, sw := range m.Switches {
		switches = append(switches, sw)
	}
	m.mu.RUnlock()

	servers := make(map[string]*Switch)
	for _, sw := range switches {
		sw.mu.RLock()
		if sw.VTP.Mode == VTPServer && sw.VTP.Domain != "" {
			servers[sw.VTP.Domain] = sw
		}
		sw.mu.RUnlock()
	}

	for _, sw := range switches {
		sw.mu.RLock()
		mode := sw.VTP.Mode
		domain := sw.VTP.Domain
		sw.mu.RUnlock()
		if mode != VTPClient || domain == "" {
			continue
		}
		server, ok := servers[domain]
		if !ok {
			continue
		}
		m.syncVTPFromServer(server, sw)
	}
}

func (m *Manager) syncVTPFromServer(server, client *Switch) {
	server.mu.RLock()
	vlans := make(map[pdu.VLANID]bool)
	names := make(map[pdu.VLANID]string)
	for v := range server.VLANs {
		vlans[v] = true
	}
	for v, n := range server.VLANNames {
		names[v] = n
	}
	rev := server.VTP.Revision
	server.mu.RUnlock()

	client.mu.Lock()
	defer client.mu.Unlock()
	if rev <= client.VTP.Revision {
		return
	}
	for v := range vlans {
		client.VLANs[v] = true
	}
	if client.VLANNames == nil {
		client.VLANNames = make(map[pdu.VLANID]string)
	}
	for v, n := range names {
		client.VLANNames[v] = n
	}
	client.VTP.Revision = rev
	m.LogEvent(EventProtocol, client.ID, "", fmt.Sprintf("VTP updated from domain %s rev %d", client.VTP.Domain, rev), nil)
}

// ListVLANs returns VLAN database for a switch.
func (sw *Switch) ListVLANs() []VLANEntry {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	entries := make([]VLANEntry, 0, len(sw.VLANs))
	for v := range sw.VLANs {
		name := fmt.Sprintf("VLAN%04d", v)
		if sw.VLANNames != nil && sw.VLANNames[v] != "" {
			name = sw.VLANNames[v]
		}
		entries = append(entries, VLANEntry{ID: v, Name: name})
	}
	return entries
}

// SetVoiceVLAN configures voice VLAN on an access port (Cisco phone + PC).
func (sw *Switch) SetVoiceVLAN(portID string, dataVLAN, voiceVLAN pdu.VLANID) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensurePort(portID)
	sw.Ports[portID].Mode = PortModeAccess
	sw.Ports[portID].AccessVLAN = dataVLAN
	sw.Ports[portID].VoiceVLAN = voiceVLAN
	sw.Ports[portID].VoiceEnabled = true
	sw.VLANs[dataVLAN] = true
	sw.VLANs[voiceVLAN] = true
}

// SetQoSTrust marks a port for VoIP QoS priority.
func (sw *Switch) SetQoSTrust(portID string, priority int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensurePort(portID)
	sw.Ports[portID].QoSPriority = priority
}
