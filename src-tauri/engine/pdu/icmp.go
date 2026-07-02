package pdu

// ICMP types.
const (
	ICMPEchoReply   uint8 = 0
	ICMPEchoRequest uint8 = 8
)

// ICMPPacket represents an Internet Control Message Protocol PDU.
type ICMPPacket struct {
	Type     uint8  `json:"type"`
	Code     uint8  `json:"code"`
	Checksum uint16 `json:"checksum"`
	ID       uint16 `json:"id"`
	Sequence uint16 `json:"sequence"`
	Data     []byte `json:"data,omitempty"`
}

// NewEchoRequest builds an ICMP echo request.
func NewEchoRequest(id, seq uint16, data []byte) *ICMPPacket {
	return &ICMPPacket{
		Type:     ICMPEchoRequest,
		Code:     0,
		ID:       id,
		Sequence: seq,
		Data:     data,
	}
}

// NewEchoReply builds an ICMP echo reply from a request.
func NewEchoReply(req *ICMPPacket) *ICMPPacket {
	return &ICMPPacket{
		Type:     ICMPEchoReply,
		Code:     0,
		ID:       req.ID,
		Sequence: req.Sequence,
		Data:     req.Data,
	}
}
