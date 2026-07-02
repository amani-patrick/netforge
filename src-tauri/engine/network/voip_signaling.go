package network

import (
	"fmt"
	"sync"
	"time"

	"netforge/engine/pdu"
)

// CallState is VoIP call lifecycle.
type CallState string

const (
	CallIdle      CallState = "idle"
	CallRinging   CallState = "ringing"
	CallConnected CallState = "connected"
	CallEnded     CallState = "ended"
)

// VoIPCall is an active or historical call.
type VoIPCall struct {
	ID        string
	Caller    string
	Callee    string
	Protocol  string // sip, sccp
	State     CallState
	StartedAt time.Duration
	EndedAt   time.Duration
}

// SignalingEngine processes SIP and SCCP on the manager.
type SignalingEngine struct {
	Calls map[string]*VoIPCall
	mu    sync.RWMutex
}

// NewSignalingEngine creates call tracking.
func NewSignalingEngine() *SignalingEngine {
	return &SignalingEngine{Calls: make(map[string]*VoIPCall)}
}

// HandleSIP processes a SIP signaling packet.
func (m *Manager) HandleSIP(cmID string, pkt *pdu.SIPPacket, simTime time.Duration) *pdu.SIPPacket {
	if m.signaling == nil {
		m.signaling = NewSignalingEngine()
	}
	m.mu.RLock()
	_, ok := m.CallManagers[cmID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}

	switch pkt.Method {
	case pdu.SIPRegister:
		m.LogEvent(EventProtocol, cmID, "", fmt.Sprintf("SIP REGISTER from %s", pkt.From), nil)
		return &pdu.SIPPacket{Method: pdu.SIPOK, Status: 200, From: pkt.From, To: pkt.To, CallID: pkt.CallID}

	case pdu.SIPInvite:
		callID := pkt.CallID
		if callID == "" {
			callID = fmt.Sprintf("call_%s_%d", pkt.From, simTime)
		}
		m.signaling.mu.Lock()
		m.signaling.Calls[callID] = &VoIPCall{
			ID: callID, Caller: pkt.From, Callee: pkt.To,
			Protocol: "sip", State: CallRinging, StartedAt: simTime,
		}
		m.signaling.mu.Unlock()
		m.LogEvent(EventProtocol, cmID, "", fmt.Sprintf("SIP INVITE %s -> %s", pkt.From, pkt.To), nil)
		return &pdu.SIPPacket{Method: pdu.SIPRinging, Status: 180, CallID: callID, To: pkt.To, From: pkt.From}

	case pdu.SIPAck:
		m.updateCallState(pkt.CallID, CallConnected, simTime)
		return nil

	case pdu.SIPBye:
		m.updateCallState(pkt.CallID, CallEnded, simTime)
		m.LogEvent(EventProtocol, cmID, "", fmt.Sprintf("SIP BYE call %s", pkt.CallID), nil)
		return &pdu.SIPPacket{Method: pdu.SIPOK, Status: 200, CallID: pkt.CallID}
	}
	return nil
}

// HandleSCCP processes SCCP signaling from IP phones.
func (m *Manager) HandleSCCP(cmID string, pkt *pdu.SCCPPacket, phone *VoIPPhone, simTime time.Duration) *pdu.SCCPPacket {
	if m.signaling == nil {
		m.signaling = NewSignalingEngine()
	}

	switch pkt.Type {
	case pdu.SCCPRegister:
		phone.Configure(pkt.Extension, phone.VoiceIP, phone.DataIP, pkt.DestIP, phone.VoiceVLAN)
		if cm, ok := m.CallManagers[cmID]; ok {
			cm.RegisterPhone(phone)
		}
		m.LogEvent(EventProtocol, phone.ID, "", "SCCP Register "+pkt.DeviceName, nil)
		return &pdu.SCCPPacket{Type: pdu.SCCPRegisterAck, DeviceName: pkt.DeviceName, Extension: pkt.Extension, DestIP: pkt.SourceIP}

	case pdu.SCCPCallProceed:
		callID := pkt.CallID
		m.signaling.mu.Lock()
		m.signaling.Calls[callID] = &VoIPCall{
			ID: callID, Caller: pkt.Extension, Callee: pkt.DeviceName,
			Protocol: "sccp", State: CallRinging, StartedAt: simTime,
		}
		m.signaling.mu.Unlock()
		return &pdu.SCCPPacket{Type: pdu.SCCPCallProceed, CallID: callID}

	case pdu.SCCPDisconnect:
		m.updateCallState(pkt.CallID, CallEnded, simTime)
		return &pdu.SCCPPacket{Type: pdu.SCCPDisconnect, CallID: pkt.CallID}
	}
	return nil
}

func (m *Manager) updateCallState(callID string, state CallState, simTime time.Duration) {
	if m.signaling == nil {
		return
	}
	m.signaling.mu.Lock()
	defer m.signaling.mu.Unlock()
	if c, ok := m.signaling.Calls[callID]; ok {
		c.State = state
		if state == CallEnded {
			c.EndedAt = simTime
		}
	}
}

// InitiateSIPCall starts an outbound SIP call from a phone over the dataplane.
func (m *Manager) InitiateSIPCall(phoneID, callee string) (*VoIPCall, error) {
	phone, ok := m.GetVoIPPhone(phoneID)
	if !ok {
		return nil, errDeviceNotFound(phoneID)
	}
	if !phone.Registered {
		return nil, fmt.Errorf("phone not registered")
	}
	simTime := m.SimNow()
	callID := fmt.Sprintf("sip_%s_%d", phoneID, simTime)
	invite := &pdu.SIPPacket{
		Method: pdu.SIPInvite, From: phone.Extension, To: callee,
		CallID: callID, DestIP: phone.CallManagerIP, SourceIP: phone.VoiceIP,
	}
	var cmID string
	m.mu.RLock()
	for id := range m.CallManagers {
		cmID = id
		break
	}
	m.mu.RUnlock()
	if cmID == "" {
		return nil, fmt.Errorf("no call manager")
	}
	m.HandleSIP(cmID, invite, simTime)
	if err := m.SendSIPOnWire(phoneID, invite); err != nil {
		return nil, err
	}
	if m.signaling == nil {
		return nil, fmt.Errorf("signaling not initialized")
	}
	m.signaling.mu.RLock()
	call := m.signaling.Calls[callID]
	m.signaling.mu.RUnlock()
	if call == nil {
		return nil, fmt.Errorf("call not created")
	}
	return call, nil
}

// GetActiveCalls returns current VoIP calls.
func (m *Manager) GetActiveCalls() []*VoIPCall {
	if m.signaling == nil {
		return nil
	}
	m.signaling.mu.RLock()
	defer m.signaling.mu.RUnlock()
	out := make([]*VoIPCall, 0, len(m.signaling.Calls))
	for _, c := range m.signaling.Calls {
		out = append(out, c)
	}
	return out
}

// SendSCCPRegister simulates phone boot registering to CM over the wire.
func (m *Manager) SendSCCPRegister(phoneID, cmID string) error {
	phone, ok := m.GetVoIPPhone(phoneID)
	if !ok {
		return errDeviceNotFound(phoneID)
	}
	cm, ok := m.CallManagers[cmID]
	if !ok {
		return errDeviceNotFound(cmID)
	}
	pkt := &pdu.SCCPPacket{
		Type: pdu.SCCPRegister, DeviceName: phoneID,
		Extension: phone.Extension, DestIP: cm.IP, SourceIP: phone.VoiceIP,
	}
	if err := m.SendSCCPOnWire(phoneID, pkt); err != nil {
		return err
	}
	reply := m.HandleSCCP(cmID, pkt, phone, m.SimNow())
	if reply == nil || reply.Type != pdu.SCCPRegisterAck {
		return fmt.Errorf("SCCP registration failed")
	}
	return nil
}
