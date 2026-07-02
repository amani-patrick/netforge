package network_test

import (
	"testing"

	"netforge/engine/network"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

func TestACLPermitDeny(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddACLRule("TEST", network.ACLRule{
		Action: network.ACLPermit, Protocol: "icmp",
		SrcNet: "any", DstNet: "any",
	})
	rtr.AddACLRule("DENY", network.ACLRule{
		Action: network.ACLDeny, Protocol: "ip",
		SrcNet: "10.0.0.0/8", DstNet: "any",
	})

	icmpPkt := &pdu.IPv4Packet{Protocol: pdu.ProtoICMP, SourceIP: "1.1.1.1", DestinationIP: "2.2.2.2"}
	if !rtr.EvaluateACL("TEST", icmpPkt) {
		t.Fatal("expected icmp permit")
	}
	deniedPkt := &pdu.IPv4Packet{Protocol: pdu.ProtoTCP, SourceIP: "10.1.1.1", DestinationIP: "8.8.8.8"}
	if rtr.EvaluateACL("DENY", deniedPkt) {
		t.Fatal("expected deny")
	}
}

func TestNATPAT(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddInterface("Gi0/0", "192.168.1.1", "255.255.255.0", "00:00:00:00:00:01")
	rtr.AddInterface("Gi0/1", "203.0.113.1", "255.255.255.252", "00:00:00:00:00:02")
	rtr.MarkNATInside("Gi0/0")
	rtr.EnableNATOverload("Gi0/1")

	pkt := &pdu.IPv4Packet{SourceIP: "192.168.1.10", DestinationIP: "8.8.8.8"}
	out := rtr.TranslateOutbound("Gi0/1", pkt)
	if out.SourceIP == pkt.SourceIP {
		t.Fatal("expected NAT translation")
	}
	if out.SourceIP != "203.0.113.1" {
		t.Fatalf("expected outside IP, got %s", out.SourceIP)
	}
}

func TestVLANTrunkIsolation(t *testing.T) {
	sw := network.NewSwitch("sw1")
	sw.RegisterPort("Fa0/1")
	sw.RegisterPort("Fa0/2")
	sw.SetPortAccessVLAN("Fa0/1", 10)
	sw.SetPortAccessVLAN("Fa0/2", 20)

	arp, _ := pdu.NewARPFrame(pdu.MACBroadcast, "00:11:11:11:11:11", &pdu.ARPPacket{
		Operation: pdu.ArpRequest, SenderMAC: "00:11:11:11:11:11",
		SenderIP: "192.168.10.10", TargetIP: "192.168.10.1",
	})
	wire := &pdu.WireFrame{Frame: arp}

	out := sw.HandleIncomingFrame("Fa0/1", wire, 0)
	if len(out) != 0 {
		t.Fatal("VLAN 10 broadcast should not reach VLAN 20 port")
	}
}

func TestDHCPPoolAssignment(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddInterface("Gi0/0", "192.168.1.1", "255.255.255.0", "00:00:00:00:00:01")
	rtr.AddDHCPPool(network.DHCPPool{
		Name: "LAN", Network: "192.168.1.0/24",
		DefaultRouter: "192.168.1.1", DNSServer: "8.8.8.8",
	})

	discover := &pdu.DHCPPacket{
		MessageType: pdu.DHCPDiscover, ClientMAC: "00:AA:BB:CC:DD:EE",
	}
	offer := rtr.HandleDHCPDiscover(discover, "Gi0/0")
	if offer == nil {
		t.Fatal("expected DHCP offer")
	}
	if offer.YourIP == "" {
		t.Fatal("expected assigned IP")
	}
}

func TestEIGRPRouteInstall(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.EnableEigrp(100)
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")

	update := []protocol.EigrpRoute{{Network: "172.16.0.0/16", Metric: 100}}
	routes := rtr.Eigrp.ProcessUpdate(update, "10.0.0.2", "Gi0/0", 0)
	if len(routes) == 0 {
		t.Fatal("expected eigrp route")
	}
	_ = rtr.AddRoute(routes[0].Network, routes[0].NextHop, routes[0].Interface, routes[0].Metric, protocol.RouteEIGRP, protocol.AdminDistEIGRP)
	_, ok := rtr.MatchRoute("172.16.5.5")
	if !ok {
		t.Fatal("expected eigrp route in table")
	}
}

func TestInterfaceShutdown(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")
	if !rtr.IsInterfaceUp("Gi0/0") {
		t.Fatal("expected interface up")
	}
	rtr.SetInterfaceShutdown("Gi0/0", true)
	if rtr.IsInterfaceUp("Gi0/0") {
		t.Fatal("expected interface down")
	}
}

func TestSubInterface(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")
	sub := rtr.AddSubInterface("Gi0/0", 10, "192.168.10.1", "255.255.255.0")
	if sub != "Gi0/0.10" {
		t.Fatalf("unexpected subinterface id: %s", sub)
	}
	if !rtr.OwnsIP("192.168.10.1") {
		t.Fatal("expected subinterface IP on router")
	}
}

func TestDNSLookup(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddDNSRecord("server.local", "192.168.1.50")
	ip, ok := rtr.LookupDNS("server.local")
	if !ok || ip != "192.168.1.50" {
		t.Fatal("expected DNS record")
	}
}

func TestSTPBlocking(t *testing.T) {
	sw1 := network.NewSwitch("sw_a")
	sw2 := network.NewSwitch("sw_z")
	sw1.RegisterPort("Fa0/1")
	sw1.RegisterPort("Fa0/2")
	sw2.RegisterPort("Fa0/1")
	switches := []*network.Switch{sw1, sw2}
	sw1.RunSTP(switches)
	if sw1.RootBridge != "sw_a" {
		t.Fatalf("expected sw_a as root, got %s", sw1.RootBridge)
	}
}
