package network_test

import (
	"testing"
	"time"

	"netforge/engine"
	"netforge/engine/network"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

func TestPingHostToHostViaRouter(t *testing.T) {
	sched := engine.NewScheduler()
	mgr := network.NewManager()
	mgr.SetScheduler(sched)

	// Topology: HostA -- Switch -- Router -- HostB
	rtr := mgr.AddRouter("r1")
	rtr.AddInterface("GigabitEthernet0/1", "192.168.1.1", "255.255.255.0", "00:AA:BB:CC:DD:01")

	_ = mgr.AddSwitch("sw1")

	hostA := mgr.AddHost("pc1")
	hostA.Configure("192.168.1.10", "255.255.255.0", "192.168.1.1", "00:11:22:33:44:01")

	hostB := mgr.AddHost("pc2")
	hostB.Configure("192.168.2.10", "255.255.255.0", "192.168.2.1", "00:11:22:33:44:02")

	mgr.AddLink(network.TopologyLink{ID: "l1", SourceNodeID: "pc1", SourcePortID: "FastEthernet0", TargetNodeID: "sw1", TargetPortID: "Fa0/1"})
	mgr.AddLink(network.TopologyLink{ID: "l2", SourceNodeID: "sw1", SourcePortID: "Fa0/2", TargetNodeID: "r1", TargetPortID: "GigabitEthernet0/1"})
	mgr.AddLink(network.TopologyLink{ID: "l3", SourceNodeID: "r1", SourcePortID: "GigabitEthernet0/0", TargetNodeID: "pc2", TargetPortID: "FastEthernet0"})

	// Assign router LAN for host B side
	rtr.AddInterface("GigabitEthernet0/0", "192.168.2.1", "255.255.255.0", "00:AA:BB:CC:DD:02")

	if err := mgr.StartPing("pc1", "192.168.2.10", "test1"); err != nil {
		t.Fatalf("StartPing failed: %v", err)
	}

	// Process simulation events until ping completes or timeout
	for i := 0; i < 5000; i++ {
		event, ok := sched.Step()
		if ok {
			switch event.Type {
			case engine.EventPacketRx, engine.EventTimerICMP:
				mgr.HandleSimulationEvent(event)
			}
		}
		results := mgr.DrainPingResults()
		if len(results) > 0 {
			if !results[0].Success {
				t.Fatalf("ping failed: %s", results[0].Message)
			}
			return
		}
	}

	t.Fatal("ping did not complete within event budget")
}

func TestStaticRoute(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")

	err := rtr.AddRoute("172.16.0.0/16", "10.0.0.2", "Gi0/0", 1, protocol.RouteStatic, protocol.AdminDistStatic)
	if err != nil {
		t.Fatalf("AddRoute failed: %v", err)
	}

	entry, ok := rtr.MatchRoute(pdu.IPAddress("172.16.5.5"))
	if !ok {
		t.Fatal("expected static route match")
	}
	if entry.Protocol != protocol.RouteStatic {
		t.Fatalf("expected static protocol, got %s", entry.Protocol)
	}
}

func TestSwitchFlooding(t *testing.T) {
	sw := network.NewSwitch("sw1")
	sw.RegisterPort("Fa0/1")
	sw.RegisterPort("Fa0/2")

	arp, _ := pdu.NewARPFrame(pdu.MACBroadcast, "00:11:11:11:11:11", &pdu.ARPPacket{
		Operation: pdu.ArpRequest,
		SenderMAC: "00:11:11:11:11:11",
		SenderIP:  "192.168.1.10",
		TargetIP:  "192.168.1.1",
	})
	wire := &pdu.WireFrame{Frame: arp}

	out := sw.HandleIncomingFrame("Fa0/1", wire, time.Duration(0))
	if len(out) != 1 {
		t.Fatalf("expected flood to 1 port, got %d", len(out))
	}
	if _, ok := out["Fa0/2"]; !ok {
		t.Fatal("expected frame flooded to Fa0/2")
	}
}

func TestTopologyPersistence(t *testing.T) {
	mgr := network.NewManager()
	r := mgr.AddRouter("r1")
	r.AddInterface("Gi0/0", "192.168.1.1", "255.255.255.0", "00:00:00:00:00:01")
	mgr.AddHost("pc1")
	mgr.AddSwitch("sw1")
	mgr.AddLink(network.TopologyLink{ID: "l1", SourceNodeID: "pc1", SourcePortID: "Fa0", TargetNodeID: "sw1", TargetPortID: "Fa0/1"})

	path := t.TempDir() + "/topo.json"
	if err := mgr.SaveTopology(path); err != nil {
		t.Fatalf("SaveTopology: %v", err)
	}

	mgr2 := network.NewManager()
	if err := mgr2.LoadTopology(path); err != nil {
		t.Fatalf("LoadTopology: %v", err)
	}
	if _, ok := mgr2.GetRouter("r1"); !ok {
		t.Fatal("router not restored")
	}
	if _, ok := mgr2.GetHost("pc1"); !ok {
		t.Fatal("host not restored")
	}
}
