package pdu

import "encoding/json"

// PayloadType identifies the L3 content inside an Ethernet frame.
type PayloadType string

const (
	PayloadARP  PayloadType = "arp"
	PayloadIPv4 PayloadType = "ipv4"
	PayloadDHCP PayloadType = "dhcp"
	PayloadDNS  PayloadType = "dns"
	PayloadCDP  PayloadType = "cdp"
)

// FramePayload is the decoded L3 content of an Ethernet frame.
type FramePayload struct {
	Type PayloadType     `json:"type"`
	ARP  *ARPPacket      `json:"arp,omitempty"`
	IP   *IPv4Packet     `json:"ip,omitempty"`
	IPv6 *IPv6Packet     `json:"ipv6,omitempty"`
	NDP  *NDPPacket      `json:"ndp,omitempty"`
	DHCP *DHCPPacket     `json:"dhcp,omitempty"`
	DNS  *DNSPacket      `json:"dns,omitempty"`
	CDP  *CDPPacket      `json:"cdp,omitempty"`
	OSPF *OSPFWirePacket `json:"ospf,omitempty"`
	RIP  *RIPWirePacket  `json:"rip,omitempty"`
	PPP  *PPPFrame       `json:"ppp,omitempty"`
	FR   *FrameRelayFrame `json:"frame_relay,omitempty"`
}

// EncodeFramePayload serializes L3 content into an Ethernet frame.
func EncodeFramePayload(frame *EthernetFrame, payload *FramePayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	frame.Payload = data
	switch payload.Type {
	case PayloadARP:
		frame.EtherType = TypeARP
	case PayloadIPv4:
		frame.EtherType = TypeIPv4
	case PayloadIPv6, PayloadNDP:
		frame.EtherType = TypeIPv6
	}
	return nil
}

// DecodeFramePayload extracts L3 content from an Ethernet frame.
func DecodeFramePayload(frame *EthernetFrame) (*FramePayload, error) {
	if len(frame.Payload) == 0 {
		return &FramePayload{}, nil
	}
	var payload FramePayload
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// NewIPv4Frame builds an Ethernet frame carrying an IPv4 packet.
func NewIPv4Frame(dstMAC, srcMAC MACAddress, ip *IPv4Packet) (*EthernetFrame, error) {
	frame := &EthernetFrame{
		DestinationMAC: dstMAC,
		SourceMAC:      srcMAC,
		EtherType:      TypeIPv4,
	}
	err := EncodeFramePayload(frame, &FramePayload{Type: PayloadIPv4, IP: ip})
	return frame, err
}

// NewARPFrame builds an Ethernet frame carrying an ARP packet.
func NewARPFrame(dstMAC, srcMAC MACAddress, arp *ARPPacket) (*EthernetFrame, error) {
	frame := &EthernetFrame{
		DestinationMAC: dstMAC,
		SourceMAC:      srcMAC,
		EtherType:      TypeARP,
	}
	err := EncodeFramePayload(frame, &FramePayload{Type: PayloadARP, ARP: arp})
	return frame, err
}
