package network

import (
	"sync"

	"netforge/engine/pdu"
)

// AccessPoint is a wireless access point.
type AccessPoint struct {
	ID       string
	SSID     string
	Channel  int
	Security string // open, wpa2
	Password string
	BSSID    pdu.MACAddress
	Clients  map[string]*WirelessClient
	mu       sync.RWMutex
}

// WirelessClient is an associated wireless station.
type WirelessClient struct {
	MAC        pdu.MACAddress
	IP         pdu.IPAddress
	IPv6       pdu.IPv6Address
	Associated bool
}

// NewAccessPoint creates a wireless AP.
func NewAccessPoint(id string) *AccessPoint {
	return &AccessPoint{
		ID: id, SSID: "NetForge-WiFi", Channel: 6, Security: "wpa2",
		BSSID: pdu.MACAddress("AA:BB:CC:DD:EE:FF"),
		Clients: make(map[string]*WirelessClient),
	}
}

// Configure sets SSID and security.
func (ap *AccessPoint) Configure(ssid, security, password string) {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	ap.SSID = ssid
	ap.Security = security
	ap.Password = password
}

// Associate authenticates and associates a wireless client.
func (ap *AccessPoint) Associate(clientMAC pdu.MACAddress, password string) bool {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.Security == "wpa2" && password != ap.Password {
		return false
	}
	ap.Clients[string(clientMAC)] = &WirelessClient{MAC: clientMAC, Associated: true}
	return true
}

// ForwardWireless bridges wireless frame to wired uplink (L2 bridge).
func (ap *AccessPoint) ForwardWireless(clientMAC pdu.MACAddress, wire *pdu.WireFrame) *pdu.WireFrame {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	if _, ok := ap.Clients[string(clientMAC)]; !ok {
		return nil
	}
	return wire
}
