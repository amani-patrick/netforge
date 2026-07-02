package pdu

// ProtoESP is IPsec Encapsulating Security Payload.
const ProtoESP uint8 = 50

// IKEPhase1State is ISAKMP SA lifecycle.
type IKEPhase1State string

const (
	IKEDown         IKEPhase1State = "down"
	IKENegotiating  IKEPhase1State = "negotiating"
	IKEActive       IKEPhase1State = "active"
)

// ESPPacket wraps an inner IPv4 packet for site-to-site VPN.
type ESPPacket struct {
	SPI        uint32      `json:"spi"`
	SeqNum     uint32      `json:"seq_num"`
	PeerIP     IPAddress   `json:"peer_ip"`
	Transform  string      `json:"transform"`
	Inner      *IPv4Packet `json:"inner"`
}
