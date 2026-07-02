package network_test

import (
	"testing"

	"netforge/engine/cli"
	"netforge/engine/network"
	"netforge/engine/pdu"
)

func TestHSRPElection(t *testing.T) {
	mgr := network.NewManager()
	r1 := mgr.AddRouter("r1")
	r2 := mgr.AddRouter("r2")
	r1.AddInterface("Gi0/0", "192.168.1.2", "255.255.255.0", "00:00:00:00:00:01")
	r2.AddInterface("Gi0/0", "192.168.1.3", "255.255.255.0", "00:00:00:00:00:02")
	r1.ConfigureHSRP("Gi0/0", 1, "192.168.1.1", 110, true)
	r2.ConfigureHSRP("Gi0/0", 1, "192.168.1.1", 100, true)
	mgr.RunHSRPElection()
	if !r1.OwnsHSRPVirtualIP("192.168.1.1") {
		t.Fatal("expected r1 as HSRP active (higher priority)")
	}
}

func TestVTPPropagation(t *testing.T) {
	mgr := network.NewManager()
	srv := mgr.AddSwitch("sw_server")
	cli := mgr.AddSwitch("sw_client")
	srv.ConfigureVTP("CAMPUS", network.VTPServer)
	cli.ConfigureVTP("CAMPUS", network.VTPClient)
	srv.SetVLANName(pdu.VLANID(20), "Students")
	srv.SetVLANName(pdu.VLANID(30), "Teachers")
	mgr.PropagateVTP()
	vlans := cli.ListVLANs()
	found := false
	for _, v := range vlans {
		if v.Name == "Students" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected VTP client to receive VLAN from server")
	}
}

func TestDHCPExcluded(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddInterface("Gi0/0", "192.168.8.1", "255.255.255.0", "00:00:00:00:00:01")
	rtr.AddDHCPPool(network.DHCPPool{
		Name: "Student-block", Network: "192.168.8.0/24",
		DefaultRouter: "192.168.8.1",
	})
	rtr.AddDHCPExcludedRange("192.168.8.1", "192.168.8.10")
	discover := &pdu.DHCPPacket{MessageType: pdu.DHCPDiscover, ClientMAC: "00:AA:BB:CC:DD:EE"}
	offer := rtr.HandleDHCPDiscover(discover, "Gi0/0")
	if offer == nil {
		t.Fatal("expected DHCP offer")
	}
	if offer.YourIP == "192.168.8.5" {
		t.Fatal("expected excluded range to be skipped")
	}
}

func TestVoiceVLAN(t *testing.T) {
	sw := network.NewSwitch("sw1")
	sw.RegisterPort("Fa0/1")
	sw.SetVoiceVLAN("Fa0/1", 10, 150)
	portCfg, ok := sw.Ports["Fa0/1"]
	if !ok || !portCfg.VoiceEnabled || portCfg.VoiceVLAN != 150 {
		t.Fatal("expected voice VLAN 150 on port")
	}
}

func TestCLIHostnameAndNoShutdown(t *testing.T) {
	mgr := network.NewManager()
	mgr.AddRouter("r1")
	sess := cli.NewSessionStore()
	exec := &cli.ExecutorWithSession{Mgr: mgr, Sessions: sess}
	r := exec.Execute("r1", "configure terminal")
	if !r.Success {
		t.Fatal(r.Error)
	}
	r = exec.Execute("r1", "hostname HQ-Router")
	if !r.Success {
		t.Fatal(r.Error)
	}
	router, _ := mgr.GetRouter("r1")
	if router.GetHostname() != "HQ-Router" {
		t.Fatal("expected hostname set")
	}
}

func TestCellularGateway(t *testing.T) {
	mgr := network.NewManager()
	gw := mgr.AddCellularGateway("lte1")
	gw.ConnectLTE("Vodafone", "internet", "203.0.113.50")
	ue := mgr.AddMobileUE("phone1")
	if err := mgr.AttachMobileUE("phone1", "lte1", "10.10.10.5"); err != nil {
		t.Fatal(err)
	}
	if ue.IP != "10.10.10.5" {
		t.Fatal("expected UE IP assigned")
	}
}

func TestVoIPPhone(t *testing.T) {
	mgr := network.NewManager()
	phone := mgr.AddVoIPPhone("phone1")
	cm := mgr.AddCallManager("cucm1")
	cm.RegisterPhone(phone)
	if !phone.Registered {
		t.Fatal("expected phone registered to CM")
	}
}

func TestTraceroute(t *testing.T) {
	mgr := network.NewManager()
	r1 := mgr.AddRouter("r1")
	r2 := mgr.AddRouter("r2")
	r1.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:AA:00:00:00:01")
	r2.AddInterface("Gi0/0", "10.0.0.2", "255.255.255.252", "00:AA:00:00:00:02")
	mgr.AddLink(network.TopologyLink{ID: "l1", SourceNodeID: "r1", SourcePortID: "Gi0/0", TargetNodeID: "r2", TargetPortID: "Gi0/0"})
	pc := mgr.AddHost("pc1")
	pc.Configure("192.168.1.10", "255.255.255.0", "192.168.1.1", "00:11:00:00:00:01")
	mgr.AddLink(network.TopologyLink{ID: "l2", SourceNodeID: "pc1", SourcePortID: "FastEthernet0", TargetNodeID: "r1", TargetPortID: "Gi0/1"})
	r1.AddInterface("Gi0/1", "192.168.1.1", "255.255.255.0", "00:AA:00:00:00:03")
	hops, err := mgr.StartTraceroute("pc1", "10.0.0.2", "test")
	if err != nil {
		t.Fatalf("traceroute: %v", err)
	}
	if len(hops) == 0 {
		t.Fatal("expected at least one hop")
	}
}
