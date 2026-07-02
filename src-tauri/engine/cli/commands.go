package cli

import (
	"fmt"
	"strconv"
	"strings"

	"netforge/engine/network"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// dispatchConfig applies config-mode commands with session context.
func (e *ExecutorWithSession) dispatchConfig(deviceID string, sess *Session, parts []string, line string) (Result, bool) {
	if len(parts) == 0 {
		return Result{}, false
	}
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "ip":
		if sess.Mode == ModeConfigIf && len(parts) >= 4 && parts[1] == "address" {
			iface := sess.Iface
			if r, ok := e.Mgr.GetRouter(deviceID); ok {
				mac := pdu.MACAddress(fmt.Sprintf("00:00:00:00:%02X:01", len(iface)%256))
				r.AddInterface(iface, pdu.IPAddress(parts[2]), parts[3], mac)
			}
			return Result{Success: true}, true
		}
		if len(parts) >= 3 && parts[1] == "nat" {
			if len(parts) >= 4 && parts[2] == "inside" {
				iface := sess.Iface
				if iface == "" {
					iface = "GigabitEthernet0/0"
				}
				if r, ok := e.Mgr.GetRouter(deviceID); ok {
					r.MarkNATInside(iface)
				}
				return Result{Success: true}, true
			}
			if len(parts) >= 4 && parts[2] == "outside" {
				iface := sess.Iface
				if iface == "" {
					iface = "GigabitEthernet0/0"
				}
				if r, ok := e.Mgr.GetRouter(deviceID); ok {
					r.MarkNATOutside(iface)
					if len(parts) >= 5 && parts[3] == "source" && parts[4] == "overload" {
						r.EnableNATOverload(iface)
					}
				}
				return Result{Success: true}, true
			}
		}
		if len(parts) >= 5 && parts[1] == "dhcp" {
			base := &Executor{Mgr: e.Mgr}
			return base.execDHCPExclusions(deviceID, parts), true
		}
		if len(parts) >= 4 && parts[1] == "access-group" {
			iface := sess.Iface
			if iface == "" {
				iface = "GigabitEthernet0/0"
			}
			if r, ok := e.Mgr.GetRouter(deviceID); ok {
				r.ApplyIfaceACL(iface, parts[2], parts[3])
			}
			return Result{Success: true}, true
		}

	case "hostname":
		if len(parts) < 2 {
			return Result{Error: "% Incomplete command.", Success: false}, true
		}
		if r, ok := e.Mgr.GetRouter(deviceID); ok {
			r.SetHostname(parts[1])
		}
		return Result{Success: true}, true

	case "no":
		return e.execNo(deviceID, sess, parts)

	case "shutdown":
		iface := sess.Iface
		if iface == "" {
			iface = "GigabitEthernet0/0"
		}
		if r, ok := e.Mgr.GetRouter(deviceID); ok {
			r.SetInterfaceShutdown(iface, true)
		}
		return Result{Success: true}, true

	case "vlan":
		return e.execVLAN(deviceID, parts)

	case "vtp":
		return e.execVTP(deviceID, parts)

	case "standby":
		return e.execStandby(deviceID, sess, parts)

	case "switchport":
		return e.execSwitchport(deviceID, sess, parts)

	case "crypto":
		if sess.Mode == ModeConfigIf && len(parts) >= 3 && parts[1] == "map" {
			if r, ok := e.Mgr.GetRouter(deviceID); ok {
				r.ApplyCryptoMap(sess.Iface, parts[2])
			}
			return Result{Success: true}, true
		}

	case "network":
		if sess.Mode == ModeConfigRtr {
			return e.execNetwork(deviceID, parts)
		}

	case "traceroute", "trace":
		if len(parts) >= 2 {
			hops, err := e.Mgr.StartTraceroute(deviceID, parts[1], "cli")
			if err != nil {
				return Result{Error: err.Error(), Success: false}, true
			}
			lines := []string{fmt.Sprintf("Tracing route to %s", parts[1])}
			for _, h := range hops {
				if h.Address != "" {
					lines = append(lines, fmt.Sprintf("%d  %d ms  %s", h.Hop, int(h.RTT), h.Address))
				} else {
					lines = append(lines, fmt.Sprintf("%d  *", h.Hop))
				}
			}
			return Result{Lines: lines, Success: true}, true
		}
	}
	_ = line
	return Result{}, false
}

func (e *ExecutorWithSession) execNo(deviceID string, sess *Session, parts []string) (Result, bool) {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	switch strings.ToLower(parts[1]) {
	case "shutdown":
		iface := sess.Iface
		if iface == "" {
			iface = "GigabitEthernet0/0"
		}
		if r, ok := e.Mgr.GetRouter(deviceID); ok {
			r.SetInterfaceShutdown(iface, false)
		}
		if sw, ok := e.Mgr.GetSwitch(deviceID); ok {
			sw.SetPortUp(iface, true)
		}
		return Result{Success: true}, true
	}
	return Result{}, false
}

func (e *ExecutorWithSession) execVLAN(deviceID string, parts []string) (Result, bool) {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	vlanID, err := strconv.Atoi(parts[1])
	if err != nil {
		return Result{Error: "% Invalid VLAN ID", Success: false}, true
	}
	sw, ok := e.Mgr.GetSwitch(deviceID)
	if !ok {
		return Result{Error: "not a switch", Success: false}, true
	}
	sw.CreateVLAN(pdu.VLANID(vlanID))
	if len(parts) >= 4 && parts[2] == "name" {
		sw.SetVLANName(pdu.VLANID(vlanID), parts[3])
		e.Mgr.PropagateVTP()
	}
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execVTP(deviceID string, parts []string) (Result, bool) {
	if len(parts) < 3 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	sw, ok := e.Mgr.GetSwitch(deviceID)
	if !ok {
		return Result{Error: "not a switch", Success: false}, true
	}
	switch parts[1] {
	case "mode":
		sw.ConfigureVTP(sw.VTP.Domain, network.VTPMode(parts[2]))
	case "domain":
		sw.ConfigureVTP(parts[2], sw.VTP.Mode)
		e.Mgr.PropagateVTP()
	}
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execStandby(deviceID string, sess *Session, parts []string) (Result, bool) {
	if len(parts) < 4 || parts[2] != "ip" {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	groupID, _ := strconv.Atoi(parts[1])
	vip := pdu.IPAddress(parts[3])
	iface := sess.Iface
	if iface == "" {
		iface = "GigabitEthernet0/0"
	}
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	r.ConfigureHSRP(iface, groupID, vip, 100, true)
	e.Mgr.RunHSRPElection()
	return Result{Lines: []string{fmt.Sprintf("HSRP group %d configured on %s", groupID, iface)}, Success: true}, true
}

func (e *ExecutorWithSession) execSwitchport(deviceID string, sess *Session, parts []string) (Result, bool) {
	sw, ok := e.Mgr.GetSwitch(deviceID)
	if !ok {
		return Result{Error: "not a switch", Success: false}, true
	}
	port := sess.Iface
	if port == "" {
		port = "Fa0/1"
	}
	if len(parts) >= 4 && parts[1] == "access" && parts[2] == "vlan" {
		vlan, _ := strconv.Atoi(parts[3])
		sw.SetPortAccessVLAN(port, pdu.VLANID(vlan))
		return Result{Success: true}, true
	}
	if len(parts) >= 3 && parts[1] == "mode" && parts[2] == "trunk" {
		sw.SetPortTrunk(port, pdu.VLANDefault, nil)
		return Result{Success: true}, true
	}
	if len(parts) >= 5 && parts[1] == "trunk" && parts[2] == "allowed" && parts[3] == "vlan" {
		vlans := parseVLANList(parts[4])
		sw.SetPortTrunk(port, pdu.VLANDefault, vlans)
		return Result{Success: true}, true
	}
	if len(parts) >= 5 && parts[1] == "trunk" && parts[2] == "native" && parts[3] == "vlan" {
		native, _ := strconv.Atoi(parts[4])
		cfg := sw.GetPortConfig(port)
		allowed := cfg.AllowedVLANs
		sw.SetPortTrunk(port, pdu.VLANID(native), allowed)
		return Result{Success: true}, true
	}
	if len(parts) >= 4 && parts[1] == "voice" && parts[2] == "vlan" {
		voice, _ := strconv.Atoi(parts[3])
		data := pdu.VLANID(1)
		sw.SetVoiceVLAN(port, data, pdu.VLANID(voice))
		return Result{Success: true}, true
	}
	return Result{}, false
}

func (e *ExecutorWithSession) execNetwork(deviceID string, parts []string) (Result, bool) {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	cidr := parts[1]
	area := 0
	if len(parts) >= 4 && parts[2] == "area" {
		area, _ = strconv.Atoi(parts[3])
	}
	if r.Ospf != nil {
		r.ConfigureOspfNetworks([]protocol.OspfNetwork{{CIDR: cidr, Area: area}})
	}
	return Result{Success: true}, true
}

func parseVLANList(spec string) []pdu.VLANID {
	out := make([]pdu.VLANID, 0)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.Split(part, "-")
			if len(bounds) == 2 {
				lo, _ := strconv.Atoi(bounds[0])
				hi, _ := strconv.Atoi(bounds[1])
				for v := lo; v <= hi; v++ {
					out = append(out, pdu.VLANID(v))
				}
			}
			continue
		}
		v, _ := strconv.Atoi(part)
		out = append(out, pdu.VLANID(v))
	}
	return out
}

func (e *ExecutorWithSession) execShowExtended(deviceID string, parts []string) (Result, bool) {
	if len(parts) < 2 {
		return Result{}, false
	}
	sub := strings.ToLower(parts[1])
	switch sub {
	case "vlan":
		sw, ok := e.Mgr.GetSwitch(deviceID)
		if !ok {
			return Result{Error: "not a switch", Success: false}, true
		}
		lines := []string{"VLAN  Name                             Status"}
		for _, v := range sw.ListVLANs() {
			lines = append(lines, fmt.Sprintf("%-5d %-32s active", v.ID, v.Name))
		}
		return Result{Lines: lines, Success: true}, true
	case "vtp":
		sw, ok := e.Mgr.GetSwitch(deviceID)
		if !ok {
			return Result{Error: "not a switch", Success: false}, true
		}
		return Result{Lines: []string{
			fmt.Sprintf("VTP Domain Name: %s", sw.VTP.Domain),
			fmt.Sprintf("VTP Mode: %s", sw.VTP.Mode),
			fmt.Sprintf("Configuration Revision: %d", sw.VTP.Revision),
		}, Success: true}, true
	case "standby":
		r, ok := e.Mgr.GetRouter(deviceID)
		if !ok {
			return Result{Error: "not a router", Success: false}, true
		}
		lines := []string{"P - configured on interface"}
		for _, row := range r.FormatHSRPStatus() {
			lines = append(lines, fmt.Sprintf("%s - Group %d - State is %s - VIP %s",
				row.Interface, row.Group, row.State, row.VirtualIP))
		}
		return Result{Lines: lines, Success: true}, true
	}
	return Result{}, false
}

func (e *Executor) execDHCPExclusions(deviceID string, parts []string) Result {
	if len(parts) < 5 || parts[2] != "excluded-address" {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}
	}
	r.AddDHCPExcludedRange(parts[3], parts[4])
	return Result{Success: true}
}
