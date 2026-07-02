package pdu

import (
	"encoding/json"
)

// MACAddress represents a standard 48-bit physical hardware address.
// We use a string format (e.g., "00:0A:95:9D:68:16") for readability and easy JSON parsing.
type MACAddress string

const (
	MACBroadcast MACAddress = "FF:FF:FF:FF:FF:FF"
)

// EthernetType indicates which Layer 3 protocol is encapsulated inside the frame payload.
type EthernetType uint16

const (
	TypeIPv4 EthernetType = 0x0800
	TypeARP  EthernetType = 0x0806
	TypeOSPF EthernetType = 0x8809
)

// Well-known multicast MAC for OSPF AllSPFRouters (01:00:5E:00:00:05).
const MACOspfAllSPFRouters MACAddress = "01:00:5E:00:00:05"

// EthernetFrame represents a Layer 2 PDU.
type EthernetFrame struct {
	DestinationMAC MACAddress   `json:"dst_mac"`
	SourceMAC      MACAddress   `json:"src_mac"`
	EtherType      EthernetType `json:"ether_type"`
	VLAN           *VLANTag     `json:"vlan,omitempty"`
	Payload        []byte       `json:"-"`
	FCS            uint32       `json:"fcs"`
}

// L1Metadata contains the physical parameters needed by our Discrete Event Scheduler.
// It tracks how long a packet takes to physically leave an interface and cross a wire.
type L1Metadata struct {
	SourceNodeID string `json:"src_node_id"`
	SourcePortID string `json:"src_port_id"`
	DestNodeID   string `json:"dst_node_id"`
	DestPortID   string `json:"dst_port_id"`
	CableLength     float64 `json:"cable_length"` // In meters
	Bandwidth       int64   `json:"bandwidth"`    // In bits per second (e.g., 100000000 for 100Mbps)
	DelayMultiplier float64 `json:"delay_multiplier,omitempty"`
}

// WireFrame wraps our Layer 2 frame with Layer 1 structural data.
// This is the complete object that gets sent across our scheduler.
type WireFrame struct {
	ID          string         `json:"id"` // Unique tracking ID for UI animations
	Frame       *EthernetFrame `json:"frame"`
	Physical    *L1Metadata    `json:"physical"`
	QoSPriority int            `json:"qos_priority,omitempty"` // 0=best-effort, 1=priority queue
}

// Serialize converts the Frame into a byte array (Simulating serialization delay)
func (f *EthernetFrame) Serialize() ([]byte, error) {
	return json.Marshal(f)
}

// Deserialize parses raw bytes back into a functional Ethernet Frame
func DeserializeFrame(data []byte) (*EthernetFrame, error) {
	var frame EthernetFrame
	err := json.Unmarshal(data, &frame)
	return &frame, err
}