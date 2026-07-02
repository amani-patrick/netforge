package pdu

// DHCP message types.
const (
	DHCPDiscover uint8 = 1
	DHCPOffer    uint8 = 2
	DHCPRequest  uint8 = 3
	DHCPAck      uint8 = 5
)

// DHCPPacket represents a simplified BOOTP/DHCP PDU.
type DHCPPacket struct {
	Op          uint8     `json:"op"` // 1=request 2=reply
	MessageType uint8     `json:"message_type"`
	XID         uint32    `json:"xid"`
	ClientMAC   MACAddress `json:"client_mac"`
	ClientIP    IPAddress  `json:"client_ip"`
	YourIP      IPAddress  `json:"your_ip"`
	ServerIP    IPAddress  `json:"server_ip"`
	GatewayIP   IPAddress  `json:"gateway_ip"`
	DNSServer   IPAddress  `json:"dns_server"`
	SubnetMask  string     `json:"subnet_mask"`
}
