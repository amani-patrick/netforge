package pdu

import (
	"encoding/json"
)

// IPAddress represents a standard IPv4 address (e.g., "192.168.1.1").
type IPAddress string

// ArpOpCode defines whether the ARP packet is a Request or a Reply.
type ArpOpCode uint16

const (
	ArpRequest ArpOpCode = 1
	ArpReply   ArpOpCode = 2
)

// ARPPacket represents an Address Resolution Protocol PDU.
// Used to discover a hardware MAC address when only the IP address is known.
type ARPPacket struct {
	HardwareType uint16     `json:"hardware_type"` // Usually 1 for Ethernet
	ProtocolType uint16     `json:"protocol_type"` // Usually 0x0800 for IPv4
	Operation    ArpOpCode  `json:"operation"`     // 1 = Request, 2 = Reply
	SenderMAC    MACAddress `json:"sender_mac"`
	SenderIP     IPAddress  `json:"sender_ip"`
	TargetMAC    MACAddress `json:"target_mac"`
	TargetIP     IPAddress  `json:"target_ip"`
}

// IP protocol numbers for encapsulated payloads.
const (
	ProtoICMP uint8 = 1
	ProtoTCP  uint8 = 6
	ProtoUDP  uint8 = 17
	ProtoOSPF uint8 = 89
	ProtoDHCP uint8 = 17 // carried over UDP
	ProtoDNS  uint8 = 17
)

// Well-known VoIP/signaling UDP ports.
const (
	PortSIP  = 5060
	PortSCCP = 2000
	PortRTP  = 16384
)

// IPv4Packet represents a standard Internet Protocol version 4 PDU.
type IPv4Packet struct {
	Version       uint8        `json:"version"`  // Typically 4
	TTL           uint8        `json:"ttl"`      // Time to Live (prevents routing loops)
	Protocol      uint8        `json:"protocol"` // 1 = ICMP, 6 = TCP, 17 = UDP, 89 = OSPF
	DSCP          int          `json:"dscp,omitempty"`
	SourceIP      IPAddress    `json:"source_ip"`
	DestinationIP IPAddress    `json:"destination_ip"`
	SrcPort       int          `json:"src_port,omitempty"`
	DstPort       int          `json:"dst_port,omitempty"`
	ICMP          *ICMPPacket  `json:"icmp,omitempty"`
	SIP           *SIPPacket   `json:"sip,omitempty"`
	SCCP          *SCCPPacket  `json:"sccp,omitempty"`
	RTP           *RTPPacket   `json:"rtp,omitempty"`
	ESP           *ESPPacket   `json:"esp,omitempty"`
	Payload       []byte       `json:"payload,omitempty"` // Generic L4 data
}

// SerializeARP packs an ARP structure into a byte array so it can sit in an EthernetFrame's payload.
func (arp *ARPPacket) Serialize() ([]byte, error) {
	return json.Marshal(arp)
}

// SerializeIP packs an IP packet into a byte array.
func (ip *IPv4Packet) Serialize() ([]byte, error) {
	return json.Marshal(ip)
}