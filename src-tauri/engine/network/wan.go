package network

import (
	"fmt"
	"sync"

	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// PPPState is the PPP link phase.
type PPPState string

const (
	PPPDead PPPState = "Dead"
	PPPLCP  PPPState = "LCP"
	PPPNCP  PPPState = "NCP"
	PPPUp   PPPState = "Up"
)

// SerialInterface is a WAN serial port.
type SerialInterface struct {
	PortID    string
	Encap     string
	Bandwidth int64
	PPPState  PPPState
}

// WANLink maps Frame Relay DLCIs between endpoints.
type WANLink struct {
	ID           string
	SourceNodeID string
	SourcePortID string
	SourceDLCI   uint16
	TargetNodeID string
	TargetPortID string
	TargetDLCI   uint16
	Encap        string
}

// FRLMIStatus reflects the state of a Frame Relay PVC.
type FRLMIStatus string

const (
	FRLMIActive   FRLMIStatus = "Active"
	FRLMIInactive FRLMIStatus = "Inactive"
	FRLMIDeleted  FRLMIStatus = "Deleted"
)

// WANManager tracks serial/WAN state.
type WANManager struct {
	SerialPorts map[string]map[string]*SerialInterface
	FRLinks     []WANLink
	// lmiSeq tracks the LMI sequence number per router:port for keepalive exchange.
	lmiSeq map[string]uint8
	// pvcStatus tracks per-DLCI status per router:port.
	pvcStatus map[string]FRLMIStatus
	mu        sync.RWMutex
}

// NewWANManager creates WAN tracking.
func NewWANManager() *WANManager {
	return &WANManager{
		SerialPorts: make(map[string]map[string]*SerialInterface),
		lmiSeq:      make(map[string]uint8),
		pvcStatus:   make(map[string]FRLMIStatus),
	}
}

func frLMIKey(nodeID, portID string, dlci uint16) string {
	return fmt.Sprintf("%s:%s:%d", nodeID, portID, dlci)
}

func frPortKey(nodeID, portID string) string {
	return fmt.Sprintf("%s:%s", nodeID, portID)
}

// SendLMIEnquiry generates an LMI Status Enquiry and marks all local PVCs as needing refresh.
// Returns the sequence number sent so the caller can match the reply.
func (w *WANManager) SendLMIEnquiry(nodeID, portID string) uint8 {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := frPortKey(nodeID, portID)
	w.lmiSeq[key]++
	return w.lmiSeq[key]
}

// ProcessLMIStatus handles an incoming LMI Status message and marks the named PVCs Active.
func (w *WANManager) ProcessLMIStatus(nodeID, portID string, dlcis []uint16) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, dlci := range dlcis {
		w.pvcStatus[frLMIKey(nodeID, portID, dlci)] = FRLMIActive
	}
}

// PVCStatus returns the current LMI status for a DLCI.
func (w *WANManager) PVCStatus(nodeID, portID string, dlci uint16) FRLMIStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	st, ok := w.pvcStatus[frLMIKey(nodeID, portID, dlci)]
	if !ok {
		return FRLMIInactive
	}
	return st
}

// ActivateFRPVCs runs an LMI enquiry/status exchange for all Frame Relay links
// and marks every configured PVC as Active. Call on a timer or after AddFRLink.
func (w *WANManager) ActivateFRPVCs() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, link := range w.FRLinks {
		w.pvcStatus[frLMIKey(link.SourceNodeID, link.SourcePortID, link.SourceDLCI)] = FRLMIActive
		w.pvcStatus[frLMIKey(link.TargetNodeID, link.TargetPortID, link.TargetDLCI)] = FRLMIActive
		w.lmiSeq[frPortKey(link.SourceNodeID, link.SourcePortID)]++
		w.lmiSeq[frPortKey(link.TargetNodeID, link.TargetPortID)]++
	}
}

// ConfigureSerial sets encapsulation on a serial port.
func (w *WANManager) ConfigureSerial(routerID, portID, encap string, bandwidth int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.SerialPorts[routerID] == nil {
		w.SerialPorts[routerID] = make(map[string]*SerialInterface)
	}
	w.SerialPorts[routerID][portID] = &SerialInterface{
		PortID: portID, Encap: encap, Bandwidth: bandwidth, PPPState: PPPDead,
	}
}

// GetSerial returns the SerialInterface for a given router port, or nil.
func (w *WANManager) GetSerial(routerID, portID string) *SerialInterface {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.SerialPorts[routerID] == nil {
		return nil
	}
	return w.SerialPorts[routerID][portID]
}

// AddFRLink registers a Frame Relay virtual circuit and immediately activates the PVC
// via an LMI status exchange so both ends show the DLCI as Active.
func (w *WANManager) AddFRLink(link WANLink) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.FRLinks = append(w.FRLinks, link)
	// Mark both ends active immediately (LMI exchange simulation).
	w.pvcStatus[frLMIKey(link.SourceNodeID, link.SourcePortID, link.SourceDLCI)] = FRLMIActive
	w.pvcStatus[frLMIKey(link.TargetNodeID, link.TargetPortID, link.TargetDLCI)] = FRLMIActive
	w.lmiSeq[frPortKey(link.SourceNodeID, link.SourcePortID)]++
	w.lmiSeq[frPortKey(link.TargetNodeID, link.TargetPortID)]++
}

// ProcessPPPFrame handles PPP LCP/NCP negotiation.
func (w *WANManager) ProcessPPPFrame(routerID, portID string, ppp *pdu.PPPFrame) *pdu.PPPFrame {
	w.mu.Lock()
	defer w.mu.Unlock()
	serial := w.SerialPorts[routerID][portID]
	if serial == nil || serial.Encap != "ppp" {
		return nil
	}
	switch ppp.Stage {
	case "LCP":
		serial.PPPState = PPPLCP
		return &pdu.PPPFrame{Stage: "LCP", AuthOK: true}
	case "NCP":
		serial.PPPState = PPPNCP
		return &pdu.PPPFrame{Stage: "NCP", Protocol: 0x8021}
	case "DATA":
		serial.PPPState = PPPUp
	}
	return nil
}

// ResolveFRDLCI maps incoming DLCI to peer endpoint.
func (w *WANManager) ResolveFRDLCI(nodeID, portID string, dlci uint16) (string, string, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, link := range w.FRLinks {
		if link.SourceNodeID == nodeID && link.SourcePortID == portID && link.SourceDLCI == dlci {
			return link.TargetNodeID, link.TargetPortID, true
		}
		if link.TargetNodeID == nodeID && link.TargetPortID == portID && link.TargetDLCI == dlci {
			return link.SourceNodeID, link.SourcePortID, true
		}
	}
	return "", "", false
}

// AssignPPPSerialAddress assigns /30 after PPP is up.
func (r *Router) AssignPPPSerialAddress(portID string, linkIndex int, isFirst bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.Interfaces[portID]; exists {
		return
	}
	host := 1
	if !isFirst {
		host = 2
	}
	ip := pdu.IPAddress(fmt.Sprintf("10.255.%d.%d", linkIndex, host))
	mask := "255.255.255.252"
	mac := pdu.MACAddress(fmt.Sprintf("00:50:00:%02X:%02X", linkIndex, host))
	r.Interfaces[portID] = ip
	r.InterfaceMask[portID] = mask
	r.InterfaceMAC[portID] = mac
	_ = r.addRouteLocked(fmt.Sprintf("10.255.%d.%d/30", linkIndex, host&^3), pdu.IPAddress(""), portID, 0, protocol.RouteConnected, protocol.AdminDistConnected)
}
