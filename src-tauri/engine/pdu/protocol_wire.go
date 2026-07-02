package pdu

const (
	TypeIPv6 EthernetType = 0x86DD
)

// IPv6 multicast solicited-node MAC prefix.
const MACIPv6Multicast MACAddress = "33:33:00:00:00:01"

// ProtoICMPv6 is IPv6 next-header for ICMPv6.
const ProtoICMPv6 uint8 = 58

// NDP message types.
const (
	NDPNeighborSolicit   uint8 = 135
	NDPNeighborAdvert    uint8 = 136
	NDPRouterSolicit     uint8 = 133
)

// NDPPacket is IPv6 Neighbor Discovery.
type NDPPacket struct {
	Type       uint8       `json:"type"`
	TargetIP   IPv6Address `json:"target_ip"`
	SenderIP   IPv6Address `json:"sender_ip"`
	SenderMAC  MACAddress  `json:"sender_mac"`
	TargetMAC  MACAddress  `json:"target_mac"`
}

// UDPHeader is a minimal UDP header inside IPv4/IPv6.
type UDPHeader struct {
	SourcePort  uint16 `json:"source_port"`
	DestPort    uint16 `json:"dest_port"`
	PayloadType string `json:"payload_type"` // rip, dns
}

// OSPFWirePacket wraps OSPF for on-wire IP proto 89 delivery.
type OSPFWirePacket struct {
	Hello *OspfHelloWire `json:"hello,omitempty"`
	LSA   *RouterLSAWire `json:"lsa,omitempty"`
}

// OspfHelloWire is on-wire OSPF Hello.
type OspfHelloWire struct {
	RouterID        IPAddress   `json:"router_id"`
	NetworkMask     string      `json:"network_mask"`
	ActiveNeighbors []IPAddress `json:"active_neighbors"`
}

// RouterLSAWire is on-wire Router LSA summary.
type RouterLSAWire struct {
	RouterID string          `json:"router_id"`
	Links    []LSALinkWire   `json:"links"`
}

type LSALinkWire struct {
	ConnectedID string `json:"connected_id"`
	Cost        int    `json:"cost"`
}

// RIPWirePacket is RIP carried in UDP/520.
type RIPWirePacket struct {
	Command uint8           `json:"command"` // 2=response
	Routes  []RIPWireRoute  `json:"routes"`
}

type RIPWireRoute struct {
	Family uint16 `json:"family"` // 2=IPv4
	CIDR   string `json:"cidr"`
	Metric uint16 `json:"metric"`
}

// PPPFrame is a Point-to-Point Protocol PDU.
type PPPFrame struct {
	Protocol uint16 `json:"protocol"` // 0x0021=IPv4, 0x8021=IPCP
	Stage    string `json:"stage"`    // LCP, NCP, DATA
	Payload  []byte `json:"payload,omitempty"`
	AuthOK   bool   `json:"auth_ok"`
}

// FrameRelayFrame is a Frame Relay PDU with DLCI.
type FrameRelayFrame struct {
	DLCI    uint16 `json:"dlci"`
	Payload []byte `json:"payload,omitempty"`
}
