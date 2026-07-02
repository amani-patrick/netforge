package pdu

// SIP methods.
const (
	SIPInvite    = "INVITE"
	SIPAck       = "ACK"
	SIPBye       = "BYE"
	SIPRegister  = "REGISTER"
	SIPOK        = "200"
	SIPRinging   = "180"
	SIPBusy      = "486"
)

// SIPPacket is a simplified SIP signaling PDU.
type SIPPacket struct {
	Method   string      `json:"method"`
	From     string      `json:"from"`
	To       string      `json:"to"`
	CallID   string      `json:"call_id"`
	CSeq     int         `json:"cseq"`
	Status   int         `json:"status,omitempty"`
	Contact  string      `json:"contact,omitempty"`
	Body     string      `json:"body,omitempty"`
	SourceIP IPAddress   `json:"source_ip"`
	DestIP   IPAddress   `json:"dest_ip"`
}

// SCCPMessageType constants.
const (
	SCCPRegister    = "Register"
	SCCPRegisterAck = "RegisterAck"
	SCCPKeepAlive   = "KeepAlive"
	SCCPCallProceed = "CallProceed"
	SCCPDisconnect  = "Disconnect"
)

// SCCPPacket is Cisco Skinny Client Control Protocol.
type SCCPPacket struct {
	Type       string      `json:"type"`
	DeviceName string      `json:"device_name"`
	Extension  string      `json:"extension"`
	CallID     string      `json:"call_id,omitempty"`
	SourceIP   IPAddress   `json:"source_ip"`
	DestIP     IPAddress   `json:"dest_ip"`
}

// RTPPacket is minimal media placeholder (signaling simulation only).
type RTPPacket struct {
	Codec      string `json:"codec"` // g711ulaw, g729
	PayloadLen int    `json:"payload_len"`
	SSRC       uint32 `json:"ssrc"`
}

// QoSDSCP well-known values.
const (
	DSCPDefault = 0
	DSCPEF      = 46  // Expedited Forwarding (voice)
	DSCPAF41    = 34  // Video
)
