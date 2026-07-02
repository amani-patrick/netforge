package pdu

// DNSPacket is a simplified DNS query/response.
type DNSPacket struct {
	ID       uint16 `json:"id"`
	Query    bool   `json:"query"`
	Name     string `json:"name"`
	Type     string `json:"type"` // A, AAAA
	Response IPAddress `json:"response,omitempty"`
}
