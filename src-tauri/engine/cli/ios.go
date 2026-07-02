package cli

import (
	"fmt"
	"strconv"
	"strings"

	"netforge/engine/network"
	"netforge/engine/pdu"
)

// Executor runs IOS-like commands against the simulation manager.
type Executor struct {
	Mgr *network.Manager
}

// Result is CLI output lines.
type Result struct {
	Lines   []string `json:"lines"`
	Error   string   `json:"error,omitempty"`
	Success bool     `json:"success"`
}

// Execute parses and runs one IOS command line for a device.
func (e *Executor) Execute(deviceID, line string) Result {
	line = strings.TrimSpace(line)
	if line == "" {
		return Result{Success: true}
	}

	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "show":
		return e.execShow(deviceID, parts)
	case "ping":
		return e.execPing(deviceID, parts)
	case "ping6":
		return e.execPing6(deviceID, parts)
	case "configure", "conf":
		return Result{Lines: []string{"Enter configuration commands, one per line. End with CNTL/Z."}, Success: true}
	case "interface", "int":
		return Result{Lines: []string{"Enter interface configuration mode."}, Success: true}
	case "ip":
		return e.execIP(deviceID, parts)
	case "ipv6":
		return e.execIPv6(deviceID, parts)
	case "router":
		return e.execRouter(deviceID, parts)
	case "encapsulation":
		return e.execEncap(deviceID, parts)
	case "frame-relay":
		return e.execFrameRelay(deviceID, parts)
	case "access-list":
		return e.execACL(deviceID, parts)
	case "nat":
		return e.execNAT(deviceID, parts)
	case "hostname":
		return Result{Success: true}
	case "wireless":
		return e.execWireless(deviceID, parts)
	case "security-level":
		return Result{Success: true}
	case "nameif":
		return Result{Success: true}
	case "end", "exit":
		return Result{Lines: []string{"Returning to privileged EXEC mode."}, Success: true}
	default:
		return Result{Error: fmt.Sprintf("%% Unknown command: '%s'", line), Success: false}
	}
}

func (e *Executor) execShow(deviceID string, parts []string) Result {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	sub := strings.ToLower(parts[1])
	switch sub {
	case "ip":
		if len(parts) >= 3 && parts[2] == "route" {
			rows, err := e.Mgr.GetRouteTable(deviceID)
			if err != nil {
				return Result{Error: err.Error(), Success: false}
			}
			lines := []string{"Codes: C - connected, S - static, O - OSPF, R - RIP, D - EIGRP, B - BGP"}
			for _, r := range rows {
				lines = append(lines, fmt.Sprintf("%s %s via %s, %s", r.Protocol, r.Network, r.NextHop, r.Interface))
			}
			return Result{Lines: lines, Success: true}
		}
	case "ipv6":
		if len(parts) >= 3 && parts[2] == "route" {
			router, ok := e.Mgr.GetRouter(deviceID)
			if !ok {
				return Result{Error: "device not found", Success: false}
			}
			rows := router.FormatIPv6RouteTable()
			lines := []string{"IPv6 Routing Table"}
			for _, r := range rows {
				lines = append(lines, fmt.Sprintf("%s %s via %s, %s", r.Protocol, r.Network, r.NextHop, r.Interface))
			}
			return Result{Lines: lines, Success: true}
		}
	case "ospf":
		if len(parts) >= 3 && parts[2] == "neighbor" {
			neighbors, err := e.Mgr.GetOspfNeighbors(deviceID)
			if err != nil {
				return Result{Error: err.Error(), Success: false}
			}
			lines := []string{"Neighbor ID     State     Interface"}
			for _, n := range neighbors {
				lines = append(lines, fmt.Sprintf("%-15s %-9s %s", n.RouterID, n.State, n.Interface))
			}
			return Result{Lines: lines, Success: true}
		}
	case "cdp":
		if len(parts) >= 3 && parts[2] == "neighbors" {
			router, ok := e.Mgr.GetRouter(deviceID)
			if !ok {
				return Result{Error: "device not found", Success: false}
			}
			lines := []string{"Device ID        Local Intrfce"}
			for _, n := range router.CDPNeighbors {
				lines = append(lines, fmt.Sprintf("%-16s %s", n.DeviceID, n.LocalPort))
			}
			return Result{Lines: lines, Success: true}
		}
	case "frame-relay":
		if len(parts) >= 3 && parts[2] == "map" {
			return Result{Lines: []string{"Frame Relay MAP (simulated)"}, Success: true}
		}
	case "wireless":
		return Result{Lines: []string{"SSID associations (simulated)"}, Success: true}
	}
	return Result{Error: "% Incomplete command.", Success: false}
}

func (e *Executor) execPing(deviceID string, parts []string) Result {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	err := e.Mgr.StartPing(deviceID, parts[1], "cli")
	if err != nil {
		return Result{Error: err.Error(), Success: false}
	}
	return Result{Lines: []string{fmt.Sprintf("Sending ICMP Echos to %s...", parts[1])}, Success: true}
}

func (e *Executor) execPing6(deviceID string, parts []string) Result {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	err := e.Mgr.StartPing6(deviceID, parts[1], "cli")
	if err != nil {
		return Result{Error: err.Error(), Success: false}
	}
	return Result{Lines: []string{fmt.Sprintf("Sending ICMPv6 Echos to %s...", parts[1])}, Success: true}
}

func (e *Executor) execIP(deviceID string, parts []string) Result {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	switch parts[1] {
	case "address":
		if len(parts) < 4 {
			return Result{Error: "% Incomplete command.", Success: false}
		}
		ip := parts[2]
		mask := parts[3]
		router, ok := e.Mgr.GetRouter(deviceID)
		if !ok {
			return Result{Error: "not a router", Success: false}
		}
		router.AddInterface("GigabitEthernet0/0", pdu.IPAddress(ip), mask, pdu.MACAddress("00:00:00:00:00:01"))
		return Result{Success: true}
	case "route":
		if len(parts) < 5 {
			return Result{Error: "% Incomplete command.", Success: false}
		}
		cidr, nh, iface := parts[2], parts[3], parts[4]
		err := e.Mgr.AddStaticRoute(deviceID, cidr, nh, iface, 1)
		if err != nil {
			return Result{Error: err.Error(), Success: false}
		}
		return Result{Success: true}
	case "dhcp":
		if len(parts) >= 4 && parts[2] == "excluded-address" {
			return e.execDHCPExclusions(deviceID, parts)
		}
		if len(parts) >= 4 && parts[2] == "pool" {
			return Result{Success: true}
		}
	}
	return Result{Error: "% Incomplete command.", Success: false}
}

func (e *Executor) execIPv6(deviceID string, parts []string) Result {
	if len(parts) >= 3 && parts[1] == "address" {
		ip := parts[2]
		prefix := 64
		if len(parts) >= 4 {
			prefix, _ = strconv.Atoi(strings.TrimPrefix(parts[3], "/"))
		}
		router, ok := e.Mgr.GetRouter(deviceID)
		if !ok {
			return Result{Error: "not a router", Success: false}
		}
		router.AddIPv6Interface("GigabitEthernet0/0", pdu.IPv6Address(ip), prefix)
		return Result{Success: true}
	}
	return Result{Error: "% Incomplete command.", Success: false}
}

func (e *Executor) execRouter(deviceID string, parts []string) Result {
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	router, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}
	}
	switch parts[1] {
	case "ospf":
		pid := 1
		if len(parts) >= 3 {
			pid, _ = strconv.Atoi(parts[2])
		}
		router.EnableOspf(pid)
		return Result{Lines: []string{fmt.Sprintf("OSPF process %d enabled", pid)}, Success: true}
	case "rip":
		router.EnableRip()
		return Result{Lines: []string{"RIP enabled"}, Success: true}
	case "eigrp":
		asn := 100
		if len(parts) >= 3 {
			asn, _ = strconv.Atoi(parts[2])
		}
		router.EnableEigrp(asn)
		return Result{Success: true}
	case "bgp":
		asn := 65001
		if len(parts) >= 3 {
			asn, _ = strconv.Atoi(parts[2])
		}
		router.EnableBgp(asn)
		return Result{Success: true}
	}
	return Result{Error: "% Incomplete command.", Success: false}
}

func (e *Executor) execEncap(deviceID string, parts []string) Result {
	if len(parts) >= 2 && parts[0] == "encapsulation" {
		encap := parts[1]
		e.Mgr.ConfigureWANSerial(deviceID, "Serial0/0/0", encap, 1544000)
		return Result{Lines: []string{fmt.Sprintf("Encapsulation %s", encap)}, Success: true}
	}
	return Result{Error: "% Incomplete command.", Success: false}
}

func (e *Executor) execFrameRelay(deviceID string, parts []string) Result {
	if len(parts) >= 3 && parts[1] == "map" {
		return Result{Success: true}
	}
	return Result{Error: "% Incomplete command.", Success: false}
}

func (e *Executor) execACL(deviceID string, parts []string) Result {
	if len(parts) < 5 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}
	}
	aclName := parts[2]
	idx := 3
	if parts[1] == "extended" {
		aclName = parts[2]
		idx = 3
	} else if n, err := strconv.Atoi(parts[1]); err == nil {
		aclName = strconv.Itoa(n)
		idx = 2
	}
	if idx >= len(parts) {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	action := network.ACLPermit
	if strings.ToLower(parts[idx]) == "deny" {
		action = network.ACLDeny
		idx++
	}
	proto := "ip"
	if idx < len(parts) && parts[idx] != "any" && !strings.Contains(parts[idx], ".") {
		proto = parts[idx]
		idx++
	}
	srcNet := "any"
	if idx < len(parts) {
		srcNet = parts[idx]
		idx++
		if idx < len(parts) && strings.Contains(parts[idx], ".") {
			idx++ // wildcard
		}
	}
	dstNet := "any"
	if idx < len(parts) {
		dstNet = parts[idx]
	}
	r.AddACLRule(aclName, network.ACLRule{
		Action: action, Protocol: proto, SrcNet: srcNet, DstNet: dstNet,
	})
	return Result{Success: true}
}

func (e *Executor) execNAT(deviceID string, parts []string) Result {
	if len(parts) < 3 {
		return Result{Error: "% Incomplete command.", Success: false}
	}
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}
	}
	switch parts[1] {
	case "inside":
		if len(parts) >= 4 && parts[2] == "source" && parts[3] == "static" && len(parts) >= 6 {
			r.AddStaticNAT(pdu.IPAddress(parts[4]), pdu.IPAddress(parts[5]))
			return Result{Success: true}
		}
		if len(parts) >= 4 && parts[2] == "source" && parts[3] == "list" {
			return Result{Success: true}
		}
	case "outside":
		return Result{Success: true}
	}
	if parts[1] == "inside" && len(parts) >= 3 && parts[2] == "interface" {
		return Result{Success: true}
	}
	return Result{Error: "% Incomplete command.", Success: false}
}

func (e *Executor) execWireless(deviceID string, parts []string) Result {
	return Result{Success: true}
}
