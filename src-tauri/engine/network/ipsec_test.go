package network_test

import (
	"testing"

	"netforge/engine/network"
	"netforge/engine/pdu"
)

func TestIPsecSiteToSiteVPN(t *testing.T) {
	mgr := network.NewManager()

	hq := mgr.AddRouter("hq")
	hq.AddInterface("Gi0/0", "203.0.113.1", "255.255.255.252", "00:00:00:00:01:01")
	hq.AddInterface("Gi0/1", "10.2.0.1", "255.255.255.0", "00:00:00:00:01:02")

	branch := mgr.AddRouter("branch")
	branch.AddInterface("Gi0/0", "203.0.113.2", "255.255.255.252", "00:00:00:00:02:01")
	branch.AddInterface("Gi0/1", "10.1.0.1", "255.255.255.0", "00:00:00:00:02:02")

	mgr.AddLink(network.TopologyLink{
		ID: "wan", SourceNodeID: "hq", SourcePortID: "Gi0/0",
		TargetNodeID: "branch", TargetPortID: "Gi0/0",
	})

	psk := "cisco123"
	hq.SetISAKMPKey("203.0.113.2", psk)
	branch.SetISAKMPKey("203.0.113.1", psk)
	hq.AddTransformSet(network.IPSecTransformSet{Name: "TS", ESPAuth: "esp-sha-hmac", ESPEncrypt: "esp-aes"})
	branch.AddTransformSet(network.IPSecTransformSet{Name: "TS", ESPAuth: "esp-sha-hmac", ESPEncrypt: "esp-aes"})
	hq.AddACLRule("VPN-TRAFFIC", network.ACLRule{Action: network.ACLPermit, Protocol: "ip", SrcNet: "10.2.0.0/24", DstNet: "10.1.0.0/24"})
	branch.AddACLRule("VPN-TRAFFIC", network.ACLRule{Action: network.ACLPermit, Protocol: "ip", SrcNet: "10.1.0.0/24", DstNet: "10.2.0.0/24"})
	hq.AddCryptoMapEntry(network.CryptoMapEntry{
		MapName: "VPNMAP", Seq: 10, PeerIP: "203.0.113.2", TransformSet: "TS",
		ACLName: "VPN-TRAFFIC", LocalSubnet: "10.2.0.0/24", RemoteSubnet: "10.1.0.0/24",
	})
	branch.AddCryptoMapEntry(network.CryptoMapEntry{
		MapName: "VPNMAP", Seq: 10, PeerIP: "203.0.113.1", TransformSet: "TS",
		ACLName: "VPN-TRAFFIC", LocalSubnet: "10.1.0.0/24", RemoteSubnet: "10.2.0.0/24",
	})
	hq.ApplyCryptoMap("Gi0/0", "VPNMAP")
	branch.ApplyCryptoMap("Gi0/0", "VPNMAP")

	if err := mgr.NegotiateIKE("hq", "203.0.113.2", psk); err != nil {
		t.Fatal(err)
	}
	if !hq.HasActiveVPN() || !branch.HasActiveVPN() {
		t.Fatal("expected active IKE SA on both routers")
	}
	mgr.InstallVPNRoutes("hq")
	mgr.InstallVPNRoutes("branch")

	peer, ok := hq.MatchCryptoTunnel("Gi0/0", &pdu.IPv4Packet{
		SourceIP: "10.2.0.10", DestinationIP: "10.1.0.10", Protocol: pdu.ProtoICMP,
	})
	if !ok || peer == nil {
		t.Fatal("expected crypto map match for inter-site traffic")
	}
	esp := hq.EncapsulateESP(&pdu.IPv4Packet{
		SourceIP: "10.2.0.10", DestinationIP: "10.1.0.10", Protocol: pdu.ProtoICMP,
	}, peer, "Gi0/0")
	if esp.Protocol != pdu.ProtoESP || esp.ESP == nil {
		t.Fatal("expected ESP encapsulation")
	}
	inner := branch.DecapsulateESP(esp)
	if inner == nil || inner.DestinationIP != "10.1.0.10" {
		t.Fatal("expected decrypted inner packet")
	}
}
