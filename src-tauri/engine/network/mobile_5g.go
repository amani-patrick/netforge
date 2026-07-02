package network

import (
	"fmt"
	"sync"

	"netforge/engine/pdu"
)

// RAType is radio access technology.
type RAType string

const (
	RALTE RAType = "LTE"
	RA5GN RAType = "5G-NR"
	RA3G  RAType = "3G"
)

// NRBandConfig is 5G New Radio parameters.
type NRBandConfig struct {
	Band      string // n78, n41
	ARFCN     int    // NR-ARFCN
	SCS       int    // subcarrier spacing kHz: 15, 30, 60
	Bandwidth int    // MHz: 20, 40, 100
	MIMO      string // 2x2, 4x4
}

// FiveGCoreConfig is 5GC / SA core attachment parameters.
type FiveGCoreConfig struct {
	gNBID       string
	PLMN        string // 001-01
	NSSAI       string // SST-SD e.g. 1-000001
	PDUSession  int
	DNN         string // data network name / APN
	SliceType   int
}

// CellularGateway extended for 5G NR.
func (g *CellularGateway) Configure5GNR(band string, arfcn, scs, bandwidthMHz int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.RAT = RA5GN
	g.NR = &NRBandConfig{
		Band: band, ARFCN: arfcn, SCS: scs, Bandwidth: bandwidthMHz, MIMO: "4x4",
	}
	g.LTEBand = band
}

// Attach5GCore registers gateway to 5GC.
func (g *CellularGateway) Attach5GCore(plmn, nssai, dnn string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.FiveGC = &FiveGCoreConfig{
		PLMN: plmn, NSSAI: nssai, DNN: dnn,
		PDUSession: 1, SliceType: 1, gNBID: g.ID + "-gnb",
	}
	g.Connected = true
}

// Connect5GNR performs UE attach over 5G NR to gateway.
func (g *CellularGateway) Connect5GNR(carrier, dnn string, publicIP pdu.IPAddress) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Carrier = carrier
	g.APN = dnn
	g.PublicIP = publicIP
	g.RAT = RA5GN
	g.Connected = true
	if g.NR == nil {
		g.NR = &NRBandConfig{Band: "n78", ARFCN: 620000, SCS: 30, Bandwidth: 100, MIMO: "4x4"}
	}
	if g.FiveGC == nil {
		g.FiveGC = &FiveGCoreConfig{PLMN: "001-01", NSSAI: "1-000001", DNN: dnn, PDUSession: 1}
	}
}

// Attach5GNR attaches a UE over 5G NR.
func (ue *MobileUE) Attach5GNR(gatewayID string, ip pdu.IPAddress, band string, arfcn int) {
	ue.mu.Lock()
	defer ue.mu.Unlock()
	ue.GatewayID = gatewayID
	ue.IP = ip
	ue.Technology = "5G-NR"
	ue.RAT = RA5GN
	ue.NRBand = band
	ue.NRARFCN = arfcn
	ue.PDUSessionID = 1
	ue.Registered = true
}

// Extend CellularGateway - fields in mobile.go

// Attach5GNRUE is manager-level 5G NR attach.
func (m *Manager) Attach5GNRUE(ueID, gatewayID string, ip pdu.IPAddress, band string, arfcn int) error {
	ue, ok := m.MobileUEs[ueID]
	if !ok {
		return errDeviceNotFound(ueID)
	}
	gw, ok := m.GetCellularGateway(gatewayID)
	if !ok {
		return errDeviceNotFound(gatewayID)
	}
	gw.Connect5GNR(gw.Carrier, gw.APN, gw.PublicIP)
	ue.Attach5GNR(gatewayID, ip, band, arfcn)
	m.LogEvent(EventProtocol, ueID, "", fmt.Sprintf("5G NR attach band %s ARFCN %d", band, arfcn), nil)
	return nil
}

// Format5GStatus returns NR status for show commands.
func (g *CellularGateway) Format5GStatus() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	lines := []string{fmt.Sprintf("RAT: %s", g.RAT), fmt.Sprintf("Carrier: %s", g.Carrier)}
	if g.NR != nil {
		lines = append(lines,
			fmt.Sprintf("NR Band: %s ARFCN %d SCS %dkHz BW %dMHz MIMO %s",
				g.NR.Band, g.NR.ARFCN, g.NR.SCS, g.NR.Bandwidth, g.NR.MIMO))
	}
	if g.FiveGC != nil {
		lines = append(lines,
			fmt.Sprintf("5GC PLMN %s NSSAI %s DNN %s PDU Session %d",
				g.FiveGC.PLMN, g.FiveGC.NSSAI, g.FiveGC.DNN, g.FiveGC.PDUSession))
	}
	return lines
}

var _ = sync.Mutex{}
