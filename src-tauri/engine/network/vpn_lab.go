package network

import "netforge/engine/pdu"

// BuildVPNLab creates a site-to-site VPN reference topology (HQ + Branch + Cloud).
func (m *Manager) BuildVPNLab() map[string]interface{} {
	psk := "cisco123"

	hq := m.AddRouter("hq_vpn")
	hq.AddInterface("Gi0/0", "203.0.113.1", "255.255.255.252", "00:AA:00:00:00:01")
	hq.AddInterface("Gi0/1", "10.2.0.1", "255.255.255.0", "00:AA:00:00:01:02")

	branch := m.AddRouter("branch_vpn")
	branch.AddInterface("Gi0/0", "203.0.113.2", "255.255.255.252", "00:BB:00:00:00:01")
	branch.AddInterface("Gi0/1", "10.1.0.1", "255.255.255.0", "00:BB:00:00:01:02")

	m.AddLink(TopologyLink{
		ID: "vpn_wan", SourceNodeID: "hq_vpn", SourcePortID: "Gi0/0",
		TargetNodeID: "branch_vpn", TargetPortID: "Gi0/0", Bandwidth: 10_000_000,
	})

	hqPC := m.AddHost("hq_pc")
	hqPC.Configure("10.2.0.10", "255.255.255.0", "10.2.0.1", "00:CC:00:00:00:01")
	branchPC := m.AddHost("branch_pc")
	branchPC.Configure("10.1.0.10", "255.255.255.0", "10.1.0.1", "00:DD:00:00:00:01")

	for _, r := range []*Router{hq, branch} {
		r.AddTransformSet(IPSecTransformSet{Name: "TS", ESPAuth: "esp-sha-hmac", ESPEncrypt: "esp-aes", Mode: "tunnel"})
		r.AddISAKMPPolicy(ISAKMPPolicy{Priority: 10, Encryption: "aes", Hash: "sha", Authentication: "pre-share", Group: 2})
	}
	hq.SetISAKMPKey("203.0.113.2", psk)
	branch.SetISAKMPKey("203.0.113.1", psk)

	hq.AddACLRule("VPN-TRAFFIC", ACLRule{Action: ACLPermit, Protocol: "ip", SrcNet: "10.2.0.0/24", DstNet: "10.1.0.0/24"})
	branch.AddACLRule("VPN-TRAFFIC", ACLRule{Action: ACLPermit, Protocol: "ip", SrcNet: "10.1.0.0/24", DstNet: "10.2.0.0/24"})
	hq.AddCryptoMapEntry(CryptoMapEntry{MapName: "VPNMAP", Seq: 10, PeerIP: "203.0.113.2", TransformSet: "TS", ACLName: "VPN-TRAFFIC", LocalSubnet: "10.2.0.0/24", RemoteSubnet: "10.1.0.0/24"})
	branch.AddCryptoMapEntry(CryptoMapEntry{MapName: "VPNMAP", Seq: 10, PeerIP: "203.0.113.1", TransformSet: "TS", ACLName: "VPN-TRAFFIC", LocalSubnet: "10.1.0.0/24", RemoteSubnet: "10.2.0.0/24"})
	hq.ApplyCryptoMap("Gi0/0", "VPNMAP")
	branch.ApplyCryptoMap("Gi0/0", "VPNMAP")

	_ = m.NegotiateIKE("hq_vpn", pdu.IPAddress("203.0.113.2"), psk)
	m.InstallVPNRoutes("hq_vpn")
	m.InstallVPNRoutes("branch_vpn")

	return map[string]interface{}{
		"topology": "site-to-site-vpn",
		"routers":  []string{"hq_vpn", "branch_vpn"},
		"hosts":    []string{"hq_pc", "branch_pc"},
		"psk":      psk,
		"status":   "ike_active",
	}
}
