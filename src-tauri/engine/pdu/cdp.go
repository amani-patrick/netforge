package pdu

// CDPPacket is a Cisco Discovery Protocol neighbor advertisement.
type CDPPacket struct {
	DeviceID   string    `json:"device_id"`
	PortID     string    `json:"port_id"`
	Platform   string    `json:"platform"`
	IPAddress  IPAddress `json:"ip_address"`
	Capabilities string  `json:"capabilities"`
}
