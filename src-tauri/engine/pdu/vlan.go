package pdu

// VLANID is an 802.1Q VLAN identifier (1-4094).
type VLANID uint16

const (
	VLANDefault VLANID = 1
)

// VLANTag represents an 802.1Q Ethernet VLAN header.
type VLANTag struct {
	VID      VLANID `json:"vid"`
	Priority uint8  `json:"priority"`
}
