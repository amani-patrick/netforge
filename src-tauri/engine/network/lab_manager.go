package network

import "netforge/engine/pdu"

// ConfigureVTPOnSwitch sets VTP domain and mode via manager.
func (m *Manager) ConfigureVTPOnSwitch(switchID, domain string, mode VTPMode) error {
	sw, ok := m.GetSwitch(switchID)
	if !ok {
		return errDeviceNotFound(switchID)
	}
	sw.ConfigureVTP(domain, mode)
	m.PropagateVTP()
	return nil
}

// ConfigureHSRPOnRouter sets HSRP and runs election.
func (m *Manager) ConfigureHSRPOnRouter(routerID, portID string, groupID int, virtualIP string, priority int) error {
	r, ok := m.GetRouter(routerID)
	if !ok {
		return errDeviceNotFound(routerID)
	}
	r.ConfigureHSRP(portID, groupID, pdu.IPAddress(virtualIP), priority, true)
	m.RunHSRPElection()
	return nil
}

// ConfigureVoiceVLAN sets data + voice VLAN on a switch port.
func (m *Manager) ConfigureVoiceVLAN(switchID, portID string, dataVLAN, voiceVLAN int) error {
	sw, ok := m.GetSwitch(switchID)
	if !ok {
		return errDeviceNotFound(switchID)
	}
	sw.SetVoiceVLAN(portID, pdu.VLANID(dataVLAN), pdu.VLANID(voiceVLAN))
	return nil
}
