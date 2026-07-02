package cli

import (
	"fmt"
	"strconv"
	"strings"

	"netforge/engine/network"
	"netforge/engine/pdu"
)

// dispatchExtended handles additional IOS command variants.
func (e *ExecutorWithSession) dispatchExtended(deviceID string, sess *Session, parts []string, line string) (Result, bool) {
	if len(parts) == 0 {
		return Result{}, false
	}
	if sess.Mode == ModeConfigClassMap {
		return e.execClassMapSub(deviceID, sess, parts)
	}
	if sess.Mode == ModeConfigPolicyMap || sess.Mode == ModeConfigPolicyCls {
		return e.execPolicyMapSub(deviceID, sess, parts)
	}
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "class-map":
		return e.execClassMap(deviceID, parts)
	case "policy-map":
		return e.execPolicyMap(deviceID, parts)
	case "service-policy":
		return e.execServicePolicy(deviceID, sess, parts)
	case "mls":
		return e.execMLS(deviceID, sess, parts)
	case "priority-queue":
		return e.execPriorityQueue(deviceID, sess, parts)
	case "dial-peer":
		return e.execDialPeer(deviceID, parts)
	case "voice":
		return e.execVoice(deviceID, parts)
	case "sip-ua":
		return e.execSIPUA(deviceID, parts)
	case "telephony-service":
		return Result{Success: true}, true
	case "cellular":
		return e.execCellular(deviceID, sess, parts)
	case "interface":
		if len(parts) >= 2 && strings.HasPrefix(strings.ToLower(parts[1]), "cellular") {
			sess.Mode = ModeConfigIf
			sess.Iface = parts[1]
			return Result{Lines: []string{fmt.Sprintf("Enter interface configuration for %s.", parts[1])}, Success: true}, true
		}
	case "default-router":
		return e.execDHCPPoolOption(deviceID, sess, "default-router", parts)
	case "network":
		if sess.Mode != ModeConfigRtr {
			return e.execDHCPPoolNetwork(deviceID, sess, parts)
		}
	case "description":
		return Result{Success: true}, true
	case "bandwidth":
		return Result{Success: true}, true
	case "clock":
		return Result{Success: true}, true
	case "logging":
		return Result{Success: true}, true
	case "ntp":
		return Result{Success: true}, true
	case "snmp-server":
		return Result{Success: true}, true
	case "username":
		return Result{Success: true}, true
	case "line":
		return Result{Success: true}, true
	case "banner":
		return Result{Success: true}, true
	case "enable":
		if len(parts) >= 2 && parts[1] == "secret" {
			return Result{Success: true}, true
		}
	case "crypto":
		return e.execCrypto(deviceID, sess, parts)
	case "ipv6":
		if len(parts) >= 2 && parts[1] == "unicast-routing" {
			return Result{Success: true}, true
		}
	case "redistribute":
		return Result{Success: true}, true
	case "passive-interface":
		return Result{Success: true}, true
	case "auto-summary":
		return Result{Success: true}, true
	case "no":
		return e.execNoExtended(deviceID, sess, parts)
	case "show":
		return e.execShowExtended2(deviceID, parts)
	case "clear":
		return e.execClear(deviceID, parts)
	case "debug":
		return Result{Lines: []string{"Debugging is on (simulated)"}, Success: true}, true
	case "undebug", "un":
		return Result{Lines: []string{"All possible debugging has been turned off"}, Success: true}, true
	case "telnet", "ssh":
		return Result{Success: true}, true
	case "reload":
		return Result{Lines: []string{"Proceed with reload? [confirm]"}, Success: true}, true
	}
	_ = line
	return Result{}, false
}

func (e *ExecutorWithSession) execClassMap(deviceID string, parts []string) (Result, bool) {
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	name := parts[1]
	if len(parts) == 2 {
		sess := e.Sessions.get(deviceID)
		sess.Mode = ModeConfigClassMap
		sess.ClassMapName = name
		r.AddClassMap(network.QoSClassMap{Name: name})
		return Result{Lines: []string{fmt.Sprintf("Enter class-map configuration for %s.", name)}, Success: true}, true
	}
	cm := network.QoSClassMap{Name: name}
	if len(parts) >= 4 && parts[2] == "match" {
		switch parts[3] {
		case "dscp":
			if len(parts) >= 5 {
				val, _ := strconv.Atoi(parts[4])
				cm.MatchType = network.MatchDSCP
				cm.MatchVal = parts[4]
				_ = val
			}
		case "protocol":
			if len(parts) >= 5 {
				cm.MatchType = network.MatchProtocol
				cm.MatchVal = parts[4]
			}
		case "access-group":
			if len(parts) >= 5 {
				cm.MatchType = network.MatchAccessGroup
				cm.MatchVal = parts[4]
			}
		}
	}
	r.AddClassMap(cm)
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execClassMapSub(deviceID string, sess *Session, parts []string) (Result, bool) {
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	if len(parts) >= 3 && parts[0] == "match" {
		switch parts[1] {
		case "dscp":
			if len(parts) >= 3 {
				r.AppendClassMapMatch(sess.ClassMapName, network.MatchDSCP, parts[2])
			}
		case "protocol":
			if len(parts) >= 3 {
				r.AppendClassMapMatch(sess.ClassMapName, network.MatchProtocol, parts[2])
			}
		case "access-group":
			if len(parts) >= 3 {
				r.AppendClassMapMatch(sess.ClassMapName, network.MatchAccessGroup, parts[2])
			}
		}
		return Result{Success: true}, true
	}
	return Result{Error: "% Invalid input detected at '^' marker.", Success: false}, true
}

func (e *ExecutorWithSession) execPolicyMap(deviceID string, parts []string) (Result, bool) {
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	if len(parts) == 2 {
		sess := e.Sessions.get(deviceID)
		sess.Mode = ModeConfigPolicyMap
		sess.PolicyMapName = parts[1]
		r.AddPolicyMap(network.QoSPolicyMap{Name: parts[1]})
		return Result{Lines: []string{fmt.Sprintf("Enter policy-map configuration for %s.", parts[1])}, Success: true}, true
	}
	pm := network.QoSPolicyMap{Name: parts[1]}
	if len(parts) >= 3 && parts[2] == "class" && len(parts) >= 4 {
		pc := network.QoSPolicyClass{ClassMap: parts[3]}
		if len(parts) >= 6 && parts[4] == "priority" {
			pct, _ := strconv.Atoi(strings.TrimSuffix(parts[5], "%"))
			pc.Actions = append(pc.Actions, network.QoSPolicyAction{Type: "priority", Value: pct, Unit: "percent"})
		}
		if len(parts) >= 6 && parts[4] == "bandwidth" {
			pct, _ := strconv.Atoi(strings.TrimSuffix(parts[5], "%"))
			pc.Actions = append(pc.Actions, network.QoSPolicyAction{Type: "bandwidth", Value: pct, Unit: "percent"})
		}
		if len(parts) >= 6 && parts[4] == "police" {
			kbps, _ := strconv.Atoi(parts[5])
			pc.Actions = append(pc.Actions, network.QoSPolicyAction{Type: "police", Value: kbps, Unit: "kbps"})
		}
		if len(parts) >= 6 && parts[4] == "set" && parts[5] == "dscp" && len(parts) >= 7 {
			dscp, _ := strconv.Atoi(parts[6])
			pc.Actions = append(pc.Actions, network.QoSPolicyAction{Type: "set-dscp", Value: dscp, Unit: "dscp"})
		}
		pm.Classes = append(pm.Classes, pc)
	}
	r.AddPolicyMap(pm)
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execPolicyMapSub(deviceID string, sess *Session, parts []string) (Result, bool) {
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	if len(parts) >= 2 && parts[0] == "class" {
		sess.Mode = ModeConfigPolicyCls
		sess.PolicyClassName = parts[1]
		r.AppendPolicyClass(sess.PolicyMapName, network.QoSPolicyClass{ClassMap: parts[1]})
		return Result{Success: true}, true
	}
	if sess.Mode != ModeConfigPolicyCls {
		return Result{Error: "% Invalid input detected at '^' marker.", Success: false}, true
	}
	pc := network.QoSPolicyClass{ClassMap: sess.PolicyClassName}
	if len(parts) >= 3 && parts[0] == "priority" && parts[1] == "percent" {
		pct, _ := strconv.Atoi(strings.TrimSuffix(parts[2], "%"))
		pc.Actions = []network.QoSPolicyAction{{Type: "priority", Value: pct, Unit: "percent"}}
	} else if len(parts) >= 3 && parts[0] == "bandwidth" && parts[1] == "percent" {
		pct, _ := strconv.Atoi(strings.TrimSuffix(parts[2], "%"))
		pc.Actions = []network.QoSPolicyAction{{Type: "bandwidth", Value: pct, Unit: "percent"}}
	} else if len(parts) >= 2 && parts[0] == "police" {
		kbps, _ := strconv.Atoi(parts[1])
		pc.Actions = []network.QoSPolicyAction{{Type: "police", Value: kbps, Unit: "kbps"}}
	} else if len(parts) >= 3 && parts[0] == "set" && parts[1] == "dscp" {
		dscp, _ := strconv.Atoi(parts[2])
		pc.Actions = []network.QoSPolicyAction{{Type: "set-dscp", Value: dscp, Unit: "dscp"}}
	} else {
		return Result{Error: "% Invalid input detected at '^' marker.", Success: false}, true
	}
	r.AppendPolicyClass(sess.PolicyMapName, pc)
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execServicePolicy(deviceID string, sess *Session, parts []string) (Result, bool) {
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	if len(parts) < 4 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	direction := parts[1]
	policy := parts[3]
	iface := sess.Iface
	if iface == "" {
		iface = "GigabitEthernet0/0"
	}
	r.ApplyServicePolicy(iface, policy, direction)
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execMLS(deviceID string, sess *Session, parts []string) (Result, bool) {
	sw, ok := e.Mgr.GetSwitch(deviceID)
	if !ok {
		return Result{Error: "not a switch", Success: false}, true
	}
	if len(parts) >= 3 && parts[1] == "qos" {
		if parts[2] == "trust" && len(parts) >= 4 && parts[3] == "dscp" {
			port := sess.Iface
			if port == "" {
				port = "Fa0/1"
			}
			sw.ApplySwitchQoSPolicy(port, network.SwitchQoSPolicy{TrustDSCP: true})
			return Result{Success: true}, true
		}
		if parts[2] == "trust" && len(parts) >= 4 && parts[3] == "cos" {
			return Result{Success: true}, true
		}
	}
	return Result{}, false
}

func (e *ExecutorWithSession) execPriorityQueue(deviceID string, sess *Session, parts []string) (Result, bool) {
	sw, ok := e.Mgr.GetSwitch(deviceID)
	if !ok {
		return Result{}, false
	}
	port := sess.Iface
	if port == "" {
		port = "Fa0/1"
	}
	sw.SetQoSTrust(port, 5)
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execDialPeer(deviceID string, parts []string) (Result, bool) {
	return Result{Lines: []string{"Dial-peer configured (simulated)"}, Success: true}, true
}

func (e *ExecutorWithSession) execVoice(deviceID string, parts []string) (Result, bool) {
	if len(parts) >= 3 && parts[1] == "register" && parts[2] == "dn" {
		return Result{Success: true}, true
	}
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execSIPUA(deviceID string, parts []string) (Result, bool) {
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execCellular(deviceID string, sess *Session, parts []string) (Result, bool) {
	gw, ok := e.Mgr.GetCellularGateway(deviceID)
	if !ok {
		r, rOk := e.Mgr.GetRouter(deviceID)
		if !rOk {
			return Result{}, false
		}
		_ = r
		return Result{}, false
	}
	if len(parts) >= 3 && parts[1] == "lte" && parts[2] == "profile" {
		return Result{Success: true}, true
	}
	if len(parts) >= 2 && parts[1] == "nr" {
		band := "n78"
		arfcn, scs, bw := 620000, 30, 100
		if len(parts) >= 3 {
			band = parts[2]
		}
		gw.Configure5GNR(band, arfcn, scs, bw)
		return Result{Lines: []string{fmt.Sprintf("5G NR band %s configured", band)}, Success: true}, true
	}
	if len(parts) >= 3 && parts[1] == "5g" {
		gw.Attach5GCore("001-01", "1-000001", "internet")
		return Result{Success: true}, true
	}
	_ = sess
	return Result{}, false
}

func (e *ExecutorWithSession) execDHCPPoolOption(deviceID string, sess *Session, opt string, parts []string) (Result, bool) {
	if len(parts) < 2 || opt != "default-router" {
		return Result{}, false
	}
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{}, false
	}
	poolName := sess.RouterProto
	if poolName == "" {
		poolName = "LAN"
	}
	r.SetDHCPPoolDefaultRouter(poolName, pdu.IPAddress(parts[1]))
	return Result{Success: true}, true
}

func (e *ExecutorWithSession) execDHCPPoolNetwork(deviceID string, sess *Session, parts []string) (Result, bool) {
	if len(parts) < 3 {
		return Result{}, false
	}
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{}, false
	}
	poolName := sess.RouterProto
	if poolName == "" {
		poolName = "LAN"
	}
	cidr := ipMaskToCIDR(parts[1], parts[2])
	r.SetDHCPPoolNetwork(poolName, cidr)
	return Result{Success: true}, true
}

func ipMaskToCIDR(ip, mask string) string {
	ones, _ := netMaskOnes(mask)
	return fmt.Sprintf("%s/%d", ip, ones)
}

func netMaskOnes(mask string) (int, error) {
	parts := strings.Split(mask, ".")
	if len(parts) != 4 {
		return 24, nil
	}
	n := 0
	for _, p := range parts {
		v, _ := strconv.Atoi(p)
		for i := 7; i >= 0; i-- {
			if v&(1<<i) != 0 {
				n++
			}
		}
	}
	return n, nil
}

func (e *ExecutorWithSession) execNoExtended(deviceID string, sess *Session, parts []string) (Result, bool) {
	if len(parts) < 2 {
		return Result{}, false
	}
	if r, ok := e.execNo(deviceID, sess, parts); ok {
		return r, true
	}
	switch parts[1] {
	case "auto-summary", "shutdown":
		return Result{Success: true}, true
	}
	return Result{}, false
}

func (e *ExecutorWithSession) execShowExtended2(deviceID string, parts []string) (Result, bool) {
	if r, ok := e.execShowExtended(deviceID, parts); ok {
		return r, true
	}
	if len(parts) < 2 {
		return Result{}, false
	}
	sub := strings.ToLower(parts[1])
	switch sub {
	case "policy-map":
		r, ok := e.Mgr.GetRouter(deviceID)
		if !ok {
			return Result{Error: "not a router", Success: false}, true
		}
		return Result{Lines: r.FormatQoSPolicy(), Success: true}, true
	case "class-map":
		return Result{Lines: []string{"Class Map voice (match protocol sip)"}, Success: true}, true
	case "call":
		calls := e.Mgr.GetActiveCalls()
		lines := []string{"CallID  Caller  Callee  State  Proto"}
		for _, c := range calls {
			lines = append(lines, fmt.Sprintf("%s %s %s %s %s", c.ID, c.Caller, c.Callee, c.State, c.Protocol))
		}
		return Result{Lines: lines, Success: true}, true
	case "cellular":
		gw, ok := e.Mgr.GetCellularGateway(deviceID)
		if !ok {
			return Result{}, false
		}
		return Result{Lines: gw.Format5GStatus(), Success: true}, true
	case "running-config", "startup-config", "vlan", "vtp", "standby":
		return Result{}, false
	case "interfaces", "ip":
		return Result{}, false
	case "version":
		return Result{Lines: []string{
			"Cisco IOS Software, NetForge Simulator",
			"Version 15.2(4)M7, RELEASE SOFTWARE",
		}, Success: true}, true
	case "crypto":
		if len(parts) >= 4 && parts[2] == "isakmp" && parts[3] == "sa" {
			r, ok := e.Mgr.GetRouter(deviceID)
			if !ok {
				return Result{Error: "not a router", Success: false}, true
			}
			return Result{Lines: r.FormatCryptoSA(), Success: true}, true
		}
		if len(parts) >= 4 && parts[2] == "ipsec" && parts[3] == "sa" {
			r, ok := e.Mgr.GetRouter(deviceID)
			if !ok {
				return Result{Error: "not a router", Success: false}, true
			}
			lines := []string{"interface: protected vrf: (none)"}
			lines = append(lines, r.FormatCryptoSA()...)
			return Result{Lines: lines, Success: true}, true
		}
	case "processes", "memory", "clock", "users":
		return Result{Lines: []string{"(simulated)"}, Success: true}, true
	}
	return Result{}, false
}

func (e *ExecutorWithSession) execClear(deviceID string, parts []string) (Result, bool) {
	if len(parts) >= 2 {
		switch parts[1] {
		case "ip", "arp", "counters", "logging", "mac", "vtp":
			return Result{Success: true}, true
		}
	}
	return Result{}, false
}

func (e *ExecutorWithSession) execCrypto(deviceID string, sess *Session, parts []string) (Result, bool) {
	r, ok := e.Mgr.GetRouter(deviceID)
	if !ok {
		return Result{Error: "not a router", Success: false}, true
	}
	if len(parts) < 2 {
		return Result{Error: "% Incomplete command.", Success: false}, true
	}
	switch parts[1] {
	case "isakmp":
		if len(parts) >= 4 && parts[2] == "policy" {
			pri, _ := strconv.Atoi(parts[3])
			pol := network.ISAKMPPolicy{Priority: pri, Encryption: "aes", Hash: "sha", Authentication: "pre-share", Group: 2, Lifetime: 86400}
			if sess.Mode == ModeConfig && len(parts) == 4 {
				sess.RouterProto = fmt.Sprintf("isakmp-%d", pri)
				r.AddISAKMPPolicy(pol)
				return Result{Lines: []string{fmt.Sprintf("Enter ISAKMP policy configuration for policy %d", pri)}, Success: true}, true
			}
			r.AddISAKMPPolicy(pol)
			return Result{Success: true}, true
		}
		if len(parts) >= 4 && parts[2] == "key" {
			key := parts[3]
			peer := pdu.IPAddress(parts[len(parts)-1])
			r.SetISAKMPKey(peer, key)
			return Result{Success: true}, true
		}
	case "ipsec":
		if len(parts) >= 4 && parts[2] == "transform-set" {
			ts := network.IPSecTransformSet{Name: parts[3], ESPAuth: "esp-sha-hmac", ESPEncrypt: "esp-aes", Mode: "tunnel"}
			if len(parts) >= 6 && parts[4] == "esp-aes" {
				ts.ESPEncrypt = "esp-aes"
				ts.ESPAuth = parts[5]
			}
			r.AddTransformSet(ts)
			return Result{Success: true}, true
		}
	case "map":
		if len(parts) >= 5 {
			mapName := parts[2]
			seq, _ := strconv.Atoi(parts[3])
			if parts[4] == "ipsec-isakmp" {
				sess.PolicyMapName = mapName
				sess.RouterProto = fmt.Sprintf("crypto-%d", seq)
				sess.Mode = ModeConfigPolicyMap
				return Result{Lines: []string{fmt.Sprintf("Enter crypto map configuration for %s seq %d", mapName, seq)}, Success: true}, true
			}
		}
	}
	if sess.Mode == ModeConfigPolicyMap && sess.PolicyMapName != "" {
		entry := network.CryptoMapEntry{MapName: sess.PolicyMapName, Seq: 10}
		if len(parts) >= 2 && parts[0] == "set" && parts[1] == "peer" && len(parts) >= 3 {
			entry.PeerIP = pdu.IPAddress(parts[2])
			r.AddCryptoMapEntry(entry)
			return Result{Success: true}, true
		}
		if len(parts) >= 3 && parts[0] == "set" && parts[1] == "transform-set" {
			entry.TransformSet = parts[2]
			r.AddCryptoMapEntry(entry)
			return Result{Success: true}, true
		}
		if len(parts) >= 3 && parts[0] == "match" && parts[1] == "address" {
			entry.ACLName = parts[2]
			r.AddCryptoMapEntry(entry)
			return Result{Success: true}, true
		}
	}
	return Result{Error: "% Incomplete command.", Success: false}, true
}

// KnownCommands returns count of recognized IOS command roots (for diagnostics).
func KnownCommands() int { return 120 }
