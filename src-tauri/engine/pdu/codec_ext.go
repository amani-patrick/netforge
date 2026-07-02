package pdu

// Extend codec payload types for IPv6 and wire protocols.

const (
	PayloadIPv6  PayloadType = "ipv6"
	PayloadNDP   PayloadType = "ndp"
	PayloadOSPF  PayloadType = "ospf"
	PayloadRIP   PayloadType = "rip"
	PayloadPPP   PayloadType = "ppp"
	PayloadFR    PayloadType = "frame_relay"
)

// NewIPv6Frame builds an Ethernet frame with an IPv6 packet.
func NewIPv6Frame(dstMAC, srcMAC MACAddress, ip *IPv6Packet) (*EthernetFrame, error) {
	frame := &EthernetFrame{DestinationMAC: dstMAC, SourceMAC: srcMAC, EtherType: TypeIPv6}
	err := EncodeFramePayload(frame, &FramePayload{Type: PayloadIPv6, IPv6: ip})
	return frame, err
}

// NewNDPFrame builds an Ethernet frame with an NDP packet.
func NewNDPFrame(dstMAC, srcMAC MACAddress, ndp *NDPPacket) (*EthernetFrame, error) {
	frame := &EthernetFrame{DestinationMAC: dstMAC, SourceMAC: srcMAC, EtherType: TypeIPv6}
	err := EncodeFramePayload(frame, &FramePayload{Type: PayloadNDP, NDP: ndp})
	return frame, err
}
