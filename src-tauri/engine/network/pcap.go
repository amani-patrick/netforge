package network

import (
	"fmt"
	"sync"
	"time"

	"netforge/engine/pdu"
)

// CaptureEntry is one frame observed on an interface.
type CaptureEntry struct {
	SimTime   int64  `json:"sim_time_ms"`
	Direction string `json:"direction"` // in, out
	NodeID    string `json:"node_id"`
	PortID    string `json:"port_id"`
	Summary   string `json:"summary"`
	FrameID   string `json:"frame_id,omitempty"`
}

// PortCapture stores per-interface packet buffers.
type PortCapture struct {
	buffers map[string][]CaptureEntry
	maxPer  int
	mu      sync.RWMutex
}

// NewPortCapture creates a capture store.
func NewPortCapture(maxPerPort int) *PortCapture {
	if maxPerPort <= 0 {
		maxPerPort = 500
	}
	return &PortCapture{buffers: make(map[string][]CaptureEntry), maxPer: maxPerPort}
}

func captureKey(nodeID, portID string) string {
	return nodeID + ":" + portID
}

// Record stores a captured frame summary.
func (p *PortCapture) Record(simTime time.Duration, direction, nodeID, portID string, wire *pdu.WireFrame) {
	if wire == nil || wire.Frame == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	key := captureKey(nodeID, portID)
	entry := CaptureEntry{
		SimTime:   simTime.Milliseconds(),
		Direction: direction,
		NodeID:    nodeID,
		PortID:    portID,
		Summary:   summarizeFrame(wire.Frame),
		FrameID:   wire.ID,
	}
	buf := append(p.buffers[key], entry)
	if len(buf) > p.maxPer {
		buf = buf[len(buf)-p.maxPer:]
	}
	p.buffers[key] = buf
}

// GetPort returns capture entries for a specific interface.
func (p *PortCapture) GetPort(nodeID, portID string, limit int) []CaptureEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	key := captureKey(nodeID, portID)
	buf := p.buffers[key]
	if limit <= 0 || limit > len(buf) {
		limit = len(buf)
	}
	start := len(buf) - limit
	if start < 0 {
		start = 0
	}
	out := make([]CaptureEntry, limit)
	copy(out, buf[start:])
	return out
}

// GetNode returns all captures across ports on a device.
func (p *PortCapture) GetNode(nodeID string, limit int) []CaptureEntry {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var out []CaptureEntry
	for key, buf := range p.buffers {
		if len(key) > len(nodeID) && key[:len(nodeID)] == nodeID {
			out = append(out, buf...)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func summarizeFrame(frame *pdu.EthernetFrame) string {
	payload, err := pdu.DecodeFramePayload(frame)
	if err != nil || payload == nil {
		return fmt.Sprintf("eth %s -> %s", frame.SourceMAC, frame.DestinationMAC)
	}
	switch payload.Type {
	case pdu.PayloadARP:
		if payload.ARP != nil {
			return fmt.Sprintf("ARP %s -> %s", payload.ARP.SenderIP, payload.ARP.TargetIP)
		}
	case pdu.PayloadIPv4:
		if payload.IP != nil {
			return fmt.Sprintf("IP %s -> %s proto=%d", payload.IP.SourceIP, payload.IP.DestinationIP, payload.IP.Protocol)
		}
	case pdu.PayloadIPv6:
		if payload.IPv6 != nil {
			return fmt.Sprintf("IPv6 %s -> %s", payload.IPv6.SourceIP, payload.IPv6.DestinationIP)
		}
	case pdu.PayloadOSPF:
		return "OSPF"
	case pdu.PayloadRIP:
		return "RIP"
	}
	return fmt.Sprintf("eth %s -> %s", frame.SourceMAC, frame.DestinationMAC)
}

// CaptureTX records an outbound frame.
func (m *Manager) CaptureTX(nodeID, portID string, wire *pdu.WireFrame) {
	if m.pcap == nil {
		return
	}
	m.pcap.Record(m.SimNow(), "out", nodeID, portID, wire)
}

// CaptureRX records an inbound frame.
func (m *Manager) CaptureRX(nodeID, portID string, wire *pdu.WireFrame) {
	if m.pcap == nil {
		return
	}
	m.pcap.Record(m.SimNow(), "in", nodeID, portID, wire)
}

// GetPortCapture returns capture buffer for an interface.
func (m *Manager) GetPortCapture(nodeID, portID string, limit int) []CaptureEntry {
	if m.pcap == nil {
		return nil
	}
	return m.pcap.GetPort(nodeID, portID, limit)
}
