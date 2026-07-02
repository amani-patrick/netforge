package network

import (
	"sync"
	"time"

	"netforge/engine/pdu"
)

// MacTableEntry represents a single learned row in our switch's forwarding table.
type MacTableEntry struct {
	PortID   string
	LastSeen time.Duration
}

// PortMode is access or trunk.
type PortMode string

const (
	PortModeAccess PortMode = "access"
	PortModeTrunk  PortMode = "trunk"
)

// STPPortState is the spanning-tree port role.
type STPPortState string

const (
	STPForwarding STPPortState = "forwarding"
	STPBlocking   STPPortState = "blocking"
)

// SwitchPortConfig holds VLAN and STP settings for a switch port.
type SwitchPortConfig struct {
	Mode         PortMode
	AccessVLAN   pdu.VLANID
	NativeVLAN   pdu.VLANID
	AllowedVLANs []pdu.VLANID
	VoiceVLAN    pdu.VLANID
	VoiceEnabled bool
	QoSPriority  int
	STPState     STPPortState
	Up           bool
}

// MacTableKey indexes CAM by VLAN+MAC.
type MacTableKey struct {
	VLAN pdu.VLANID
	MAC  pdu.MACAddress
}

// Switch represents a Layer 2 network switch with VLAN and STP support.
type Switch struct {
	ID           string
	Ports        map[string]*SwitchPortConfig
	MacTable     map[MacTableKey]MacTableEntry
	VLANs        map[pdu.VLANID]bool
	VLANNames    map[pdu.VLANID]string
	VTP          VTPConfig
	BridgeID     string
	RootBridge   string
	BlockedPorts map[string]bool
	PortQoS      map[string]*SwitchQoSPolicy
	mu           sync.RWMutex
	AgingTime    time.Duration
}

// NewSwitch initializes a clean Layer 2 switch.
func NewSwitch(id string) *Switch {
	return &Switch{
		ID:           id,
		Ports:        make(map[string]*SwitchPortConfig),
		MacTable:     make(map[MacTableKey]MacTableEntry),
		VLANs:        map[pdu.VLANID]bool{pdu.VLANDefault: true},
		VLANNames:    make(map[pdu.VLANID]string),
		BridgeID:     id,
		RootBridge:   id,
		BlockedPorts: make(map[string]bool),
		AgingTime:    300 * time.Second,
	}
}

// RegisterPort marks a port as connected with defaults.
func (sw *Switch) RegisterPort(portID string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if _, ok := sw.Ports[portID]; !ok {
		sw.Ports[portID] = &SwitchPortConfig{
			Mode:       PortModeAccess,
			AccessVLAN: pdu.VLANDefault,
			NativeVLAN: pdu.VLANDefault,
			STPState:   STPForwarding,
			Up:         true,
		}
	}
}

// SetPortAccessVLAN configures an access port VLAN.
func (sw *Switch) SetPortAccessVLAN(portID string, vlan pdu.VLANID) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensurePort(portID)
	sw.Ports[portID].Mode = PortModeAccess
	sw.Ports[portID].AccessVLAN = vlan
	sw.VLANs[vlan] = true
}

// GetPortConfig returns a copy of port settings.
func (sw *Switch) GetPortConfig(portID string) SwitchPortConfig {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	if cfg := sw.Ports[portID]; cfg != nil {
		return *cfg
	}
	return SwitchPortConfig{Mode: PortModeAccess, AccessVLAN: pdu.VLANDefault, Up: true}
}

// SetPortTrunk configures a trunk port.
func (sw *Switch) SetPortTrunk(portID string, native pdu.VLANID, allowed []pdu.VLANID) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensurePort(portID)
	sw.Ports[portID].Mode = PortModeTrunk
	sw.Ports[portID].NativeVLAN = native
	sw.Ports[portID].AllowedVLANs = allowed
	sw.VLANs[native] = true
	for _, v := range allowed {
		sw.VLANs[v] = true
	}
}

// HasVLAN reports whether a VLAN exists on the switch.
func (sw *Switch) HasVLAN(vlan pdu.VLANID) bool {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.VLANs[vlan]
}

// CreateVLAN adds a VLAN to the switch database.
func (sw *Switch) CreateVLAN(vlan pdu.VLANID) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.VLANs[vlan] = true
}

// SetPortUp sets administrative port status.
func (sw *Switch) SetPortUp(portID string, up bool) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensurePort(portID)
	sw.Ports[portID].Up = up
}

// ensurePort creates default port config if missing.
func (sw *Switch) ensurePort(portID string) {
	if _, ok := sw.Ports[portID]; !ok {
		sw.Ports[portID] = &SwitchPortConfig{
			Mode: PortModeAccess, AccessVLAN: pdu.VLANDefault,
			NativeVLAN: pdu.VLANDefault, STPState: STPForwarding, Up: true,
		}
	}
}

// RunSTP elects a root bridge and blocks redundant ports (simplified).
func (sw *Switch) RunSTP(allSwitches []*Switch) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	root := sw.BridgeID
	for _, other := range allSwitches {
		if other.BridgeID < root {
			root = other.BridgeID
		}
	}
	sw.RootBridge = root

	if len(sw.Ports) > 2 && sw.ID > root {
		for portID := range sw.Ports {
			sw.BlockedPorts[portID] = false
		}
		var last string
		for portID := range sw.Ports {
			if portID > last {
				last = portID
			}
		}
		if last != "" {
			sw.BlockedPorts[last] = true
			sw.Ports[last].STPState = STPBlocking
		}
	}
}

func (sw *Switch) ingressVLAN(portID string, frame *pdu.EthernetFrame) pdu.VLANID {
	cfg := sw.Ports[portID]
	if cfg == nil {
		return pdu.VLANDefault
	}
	if frame.VLAN != nil && frame.VLAN.VID != 0 {
		return frame.VLAN.VID
	}
	if cfg.Mode == PortModeTrunk {
		return cfg.NativeVLAN
	}
	return cfg.AccessVLAN
}

func (sw *Switch) egressFrame(portID string, vlan pdu.VLANID, wire *pdu.WireFrame) *pdu.WireFrame {
	cfg := sw.Ports[portID]
	if cfg == nil {
		return wire
	}
	out := *wire
	outFrame := *wire.Frame
	out.Frame = &outFrame

	if cfg.Mode == PortModeTrunk {
		allowed := len(cfg.AllowedVLANs) == 0
		for _, v := range cfg.AllowedVLANs {
			if v == vlan {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil
		}
		if vlan != cfg.NativeVLAN {
			out.Frame.VLAN = &pdu.VLANTag{VID: vlan}
		} else {
			out.Frame.VLAN = nil
		}
	} else {
		if vlan != cfg.AccessVLAN {
			return nil
		}
		out.Frame.VLAN = nil
	}
	return &out
}

// HandleIncomingFrame processes frames with VLAN-aware forwarding.
func (sw *Switch) HandleIncomingFrame(incomingPort string, wireFrame *pdu.WireFrame, simTime time.Duration) map[string]*pdu.WireFrame {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	cfg := sw.Ports[incomingPort]
	if cfg == nil || !cfg.Up || sw.BlockedPorts[incomingPort] {
		return nil
	}

	frame := wireFrame.Frame
	vlan := sw.ingressVLAN(incomingPort, frame)

	sw.MacTable[MacTableKey{VLAN: vlan, MAC: frame.SourceMAC}] = MacTableEntry{
		PortID: incomingPort, LastSeen: simTime,
	}

	outbound := make(map[string]*pdu.WireFrame)

	if frame.DestinationMAC == pdu.MACBroadcast {
		return sw.floodVLAN(incomingPort, vlan, wireFrame)
	}

	entry, found := sw.MacTable[MacTableKey{VLAN: vlan, MAC: frame.DestinationMAC}]
	if found && entry.PortID != incomingPort {
		if egress := sw.egressFrame(entry.PortID, vlan, wireFrame); egress != nil {
			outbound[entry.PortID] = egress
		}
	} else if !found {
		return sw.floodVLAN(incomingPort, vlan, wireFrame)
	}

	return outbound
}

func (sw *Switch) floodVLAN(ingressPort string, vlan pdu.VLANID, wireFrame *pdu.WireFrame) map[string]*pdu.WireFrame {
	flooded := make(map[string]*pdu.WireFrame)
	for portID, cfg := range sw.Ports {
		if portID == ingressPort || !cfg.Up || sw.BlockedPorts[portID] {
			continue
		}
		if egress := sw.egressFrame(portID, vlan, wireFrame); egress != nil {
			flooded[portID] = egress
		}
	}
	return flooded
}

// Snapshot exports switch state.
func (sw *Switch) Snapshot() SwitchSnapshot {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	ports := make([]string, 0, len(sw.Ports))
	vlans := make([]int, 0, len(sw.VLANs))
	for p := range sw.Ports {
		ports = append(ports, p)
	}
	for v := range sw.VLANs {
		vlans = append(vlans, int(v))
	}
	return SwitchSnapshot{ID: sw.ID, Ports: ports, VLANs: vlans}
}

// RestoreSwitch creates a switch from a snapshot.
func RestoreSwitch(snap SwitchSnapshot) *Switch {
	sw := NewSwitch(snap.ID)
	for _, p := range snap.Ports {
		sw.RegisterPort(p)
	}
	for _, v := range snap.VLANs {
		sw.CreateVLAN(pdu.VLANID(v))
	}
	return sw
}
