package network_test

import (
	"testing"

	"netforge/engine/cli"
	"netforge/engine/network"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

func TestIPv6RouteMatch(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")
	rtr.AddIPv6Interface("Gi0/0", "2001:db8::1", 64)

	_, ok := rtr.MatchIPv6Route("2001:db8::2")
	if !ok {
		t.Fatal("expected IPv6 route match")
	}
}

func TestOSPFOnWireHello(t *testing.T) {
	mgr := network.NewManager()
	r1 := mgr.AddRouter("r1")
	r2 := mgr.AddRouter("r2")
	r1.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:AA:00:00:00:01")
	r2.AddInterface("Gi0/0", "10.0.0.2", "255.255.255.252", "00:AA:00:00:00:02")
	mgr.AddLink(network.TopologyLink{
		ID: "l1", SourceNodeID: "r1", SourcePortID: "Gi0/0",
		TargetNodeID: "r2", TargetPortID: "Gi0/0",
	})
	r1.EnableOspf(1)
	r2.EnableOspf(1)
	r1.ConfigureOspfNetworks([]protocol.OspfNetwork{{CIDR: "10.0.0.0/30", Area: 0}})
	r2.ConfigureOspfNetworks([]protocol.OspfNetwork{{CIDR: "10.0.0.0/30", Area: 0}})

	hello := r1.Ospf.GenerateHello("Gi0/0", "255.255.255.252")
	mgr.HandleWireOSPF(r2, "Gi0/0", &pdu.OSPFWirePacket{
		Hello: &pdu.OspfHelloWire{
			RouterID: hello.RouterID, NetworkMask: hello.NetworkMask,
			ActiveNeighbors: hello.ActiveNeighbors,
		},
	})
	neighbors := r2.Ospf.GetNeighbors()
	if len(neighbors) == 0 {
		t.Fatal("expected OSPF neighbor from on-wire hello")
	}
}

func TestRIPOnWireUpdate(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.EnableRip()
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")

	mgr := network.NewManager()
	mgr.AddRouter("r1")
	mgr.Routers["r1"] = rtr

	ripPkt := &pdu.RIPWirePacket{
		Command: 2,
		Routes:  []pdu.RIPWireRoute{{Family: 2, CIDR: "172.16.0.0/16", Metric: 2}},
	}
	mgr.HandleWireRIP(rtr, "Gi0/0", ripPkt, "10.0.0.2")

	_, ok := rtr.MatchRoute("172.16.5.5")
	if !ok {
		t.Fatal("expected RIP route from on-wire update")
	}
}

func TestASADenyOutside(t *testing.T) {
	asa := network.NewASAFirewall("asa1")
	asa.AddInterface("Gi0/0", "203.0.113.1", "255.255.255.252", "00:00:00:00:00:01", network.ZoneOutside)
	asa.AddInterface("Gi0/1", "10.0.0.1", "255.255.255.0", "00:00:00:00:00:02", network.ZoneInside)

	inbound := &pdu.IPv4Packet{SourceIP: "8.8.8.8", DestinationIP: "10.0.0.50", Protocol: pdu.ProtoTCP}
	if asa.InspectPacket("Gi0/0", inbound, 0) {
		t.Fatal("expected deny from outside without explicit permit")
	}

	asa.AddRule(network.ACLRule{
		Action: network.ACLPermit, Protocol: "ip",
		SrcNet: "any", DstNet: "10.0.0.0/8",
	})
	if !asa.InspectPacket("Gi0/0", inbound, 0) {
		t.Fatal("expected permit after ACL rule")
	}
}

func TestPPPNegotiation(t *testing.T) {
	wan := network.NewWANManager()
	wan.ConfigureSerial("r1", "Se0/0/0", "ppp", 1544000)

	lcp := wan.ProcessPPPFrame("r1", "Se0/0/0", &pdu.PPPFrame{Stage: "LCP"})
	if lcp == nil || !lcp.AuthOK {
		t.Fatal("expected LCP response")
	}
	ncp := wan.ProcessPPPFrame("r1", "Se0/0/0", &pdu.PPPFrame{Stage: "NCP"})
	if ncp == nil {
		t.Fatal("expected NCP response")
	}
	_ = wan.ProcessPPPFrame("r1", "Se0/0/0", &pdu.PPPFrame{Stage: "DATA"})
	serial := wan.SerialPorts["r1"]["Se0/0/0"]
	if serial.PPPState != network.PPPUp {
		t.Fatalf("expected PPP up, got %s", serial.PPPState)
	}
}

func TestWirelessWPA2(t *testing.T) {
	ap := network.NewAccessPoint("ap1")
	ap.Configure("CorpWiFi", "wpa2", "secret123")
	if ap.Associate("00:11:22:33:44:55", "wrong") {
		t.Fatal("expected auth failure")
	}
	if !ap.Associate("00:11:22:33:44:55", "secret123") {
		t.Fatal("expected association success")
	}
}

func TestIOSCLIParser(t *testing.T) {
	mgr := network.NewManager()
	rtr := mgr.AddRouter("r1")
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")

	exec := &cli.Executor{Mgr: mgr}
	result := exec.Execute("r1", "show ip route")
	if !result.Success {
		t.Fatalf("CLI failed: %s", result.Error)
	}
	if len(result.Lines) < 2 {
		t.Fatal("expected route table output")
	}

	result = exec.Execute("r1", "router ospf 1")
	if !result.Success {
		t.Fatalf("router ospf failed: %s", result.Error)
	}
	if rtr.Ospf == nil || !rtr.Ospf.Enabled {
		t.Fatal("expected OSPF enabled via CLI")
	}
}

func TestFrameRelayDLCI(t *testing.T) {
	wan := network.NewWANManager()
	wan.AddFRLink(network.WANLink{
		ID: "fr1", SourceNodeID: "r1", SourcePortID: "Se0/0/0", SourceDLCI: 100,
		TargetNodeID: "r2", TargetPortID: "Se0/0/0", TargetDLCI: 200,
	})
	node, port, ok := wan.ResolveFRDLCI("r1", "Se0/0/0", 100)
	if !ok || node != "r2" || port != "Se0/0/0" {
		t.Fatal("expected FR DLCI resolution")
	}
}
