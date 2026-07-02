package pdu

// IPv6Address is a string representation of an IPv6 address.
type IPv6Address string

// IPv6Packet is a simplified IPv6 header for dual-stack readiness.
type IPv6Packet struct {
	HopLimit      uint8         `json:"hop_limit"`
	NextHeader    uint8         `json:"next_header"`
	SourceIP      IPv6Address   `json:"source_ip"`
	DestinationIP IPv6Address   `json:"destination_ip"`
	ICMPv6        *ICMPPacket   `json:"icmpv6,omitempty"`
}
