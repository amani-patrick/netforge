package network_test

import (
	"testing"
	"time"

	"netforge/engine"
	"netforge/engine/cli"
	"netforge/engine/network"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

func TestDeviceRegistry(t *testing.T) {
	mgr := network.NewManager()
	mgr.AddRouter("r1").AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")
	mgr.AddSwitch("sw1")
	mgr.AddHost("pc1")
	mgr.AddAccessPoint("ap1")
	mgr.AddASAFirewall("asa1")

	devices := mgr.ListDevices()
	if len(devices) != 5 {
		t.Fatalf("expected 5 devices, got %d", len(devices))
	}

	dev, ok := mgr.GetDevice("r1")
	if !ok || dev.DeviceKind() != network.DeviceRouter {
		t.Fatal("expected router device")
	}
	if len(dev.PortIDs()) == 0 {
		t.Fatal("expected router ports")
	}
}

func TestEventLog(t *testing.T) {
	mgr := network.NewManager()
	mgr.LogEvent(network.EventPacketTX, "r1", "Gi0/0", "test transmit", nil)
	mgr.LogEvent(network.EventACLDeny, "r1", "Gi0/1", "denied", nil)

	entries := mgr.GetEventLog(10, "")
	if len(entries) != 2 {
		t.Fatalf("expected 2 events, got %d", len(entries))
	}
	denies := mgr.GetEventLog(10, network.EventACLDeny)
	if len(denies) != 1 {
		t.Fatal("expected 1 ACL deny event")
	}
}

func TestPortCapture(t *testing.T) {
	mgr := network.NewManager()
	frame, _ := pdu.NewARPFrame(pdu.MACBroadcast, "00:11:11:11:11:11", &pdu.ARPPacket{
		Operation: pdu.ArpRequest, SenderIP: "192.168.1.10", TargetIP: "192.168.1.1",
	})
	wire := &pdu.WireFrame{ID: "f1", Frame: frame}
	mgr.CaptureTX("r1", "Gi0/0", wire)
	mgr.CaptureRX("sw1", "Fa0/1", wire)

	tx := mgr.GetPortCapture("r1", "Gi0/0", 10)
	if len(tx) != 1 || tx[0].Direction != "out" {
		t.Fatal("expected TX capture")
	}
	rx := mgr.GetPortCapture("sw1", "Fa0/1", 10)
	if len(rx) != 1 || rx[0].Direction != "in" {
		t.Fatal("expected RX capture")
	}
}

func TestCLIConfigModes(t *testing.T) {
	mgr := network.NewManager()
	mgr.AddRouter("r1")
	sessions := cli.NewSessionStore()
	exec := &cli.ExecutorWithSession{Mgr: mgr, Sessions: sessions}

	r := exec.Execute("r1", "configure terminal")
	if !r.Success {
		t.Fatalf("configure terminal failed: %s", r.Error)
	}
	r = exec.Execute("r1", "interface GigabitEthernet0/0")
	if !r.Success {
		t.Fatalf("interface failed: %s", r.Error)
	}
	r = exec.Execute("r1", "ip address 192.168.1.1 255.255.255.255.0")
	if !r.Success {
		t.Fatalf("ip address failed: %s", r.Error)
	}
	r = exec.Execute("r1", "write memory")
	if !r.Success {
		t.Fatalf("write memory failed: %s", r.Error)
	}

	startup := mgr.GetStartupConfig("r1")
	if len(startup) == 0 {
		t.Fatal("expected startup config after write memory")
	}
	r = exec.Execute("r1", "show running-config")
	if !r.Success || len(r.Lines) < 2 {
		t.Fatal("expected running-config output")
	}
}

func TestActivityEngine(t *testing.T) {
	mgr := network.NewManager()
	rtr := mgr.AddRouter("r1")
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")
	_ = mgr.AddStaticRoute("r1", "172.16.0.0/16", "10.0.0.2", "Gi0/0", 1)

	mgr.AddActivityGoal(network.ActivityGoal{
		ID: "route_goal", Type: network.GoalRouteExists,
		Description: "Static route to 172.16.0.0/16",
		Params: map[string]string{"router_id": "r1", "network": "172.16.5.5"},
	})
	mgr.AddActivityGoal(network.ActivityGoal{
		ID: "device_goal", Type: network.GoalDeviceExists,
		Params: map[string]string{"device_id": "r1"},
	})

	results := mgr.EvaluateActivity()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if !r.Passed {
			t.Fatalf("goal %s failed: %s", r.GoalID, r.Message)
		}
	}
}

func TestTwoRouterOSPFConvergence(t *testing.T) {
	sched := engine.NewScheduler()
	mgr := network.NewManager()
	mgr.SetScheduler(sched)

	r1 := mgr.AddRouter("r1")
	r2 := mgr.AddRouter("r2")
	r1.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:AA:00:00:00:01")
	r2.AddInterface("Gi0/0", "10.0.0.2", "255.255.255.252", "00:AA:00:00:00:02")
	mgr.AddLink(network.TopologyLink{
		ID: "l1", SourceNodeID: "r1", SourcePortID: "Gi0/0",
		TargetNodeID: "r2", TargetPortID: "Gi0/0",
		Bandwidth: 100_000_000,
	})

	r1.EnableOspf(1)
	r2.EnableOspf(1)
	r1.ConfigureOspfNetworks([]protocol.OspfNetwork{{CIDR: "10.0.0.0/30", Area: 0}})
	r2.ConfigureOspfNetworks([]protocol.OspfNetwork{{CIDR: "10.0.0.0/30", Area: 0}})

	for i := 0; i < 3; i++ {
		mgr.RunOspfHelloCycle()
	}

	neighbors, err := mgr.GetOspfNeighbors("r1")
	if err != nil {
		t.Fatalf("GetOspfNeighbors: %v", err)
	}
	if len(neighbors) == 0 {
		t.Fatal("expected OSPF neighbor after hello cycles")
	}

	mgr.AddActivityGoal(network.ActivityGoal{
		ID: "ospf_adj", Type: network.GoalOspfNeighbor,
		Params: map[string]string{"router_id": "r1"},
	})
	results := mgr.EvaluateActivity()
	if len(results) == 0 || !results[0].Passed {
		t.Fatal("expected OSPF neighbor goal to pass")
	}
	_ = time.Duration(0)
}

func TestPingThreeHopIntegration(t *testing.T) {
	sched := engine.NewScheduler()
	mgr := network.NewManager()
	mgr.SetScheduler(sched)

	r1 := mgr.AddRouter("r1")
	r2 := mgr.AddRouter("r2")
	r1.AddInterface("Gi0/1", "192.168.1.1", "255.255.255.0", "00:AA:00:00:00:01")
	r1.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:AA:00:00:00:02")
	r2.AddInterface("Gi0/0", "10.0.0.2", "255.255.255.252", "00:AA:00:00:00:03")
	r2.AddInterface("Gi0/1", "192.168.2.1", "255.255.255.0", "00:AA:00:00:00:04")

	mgr.AddLink(network.TopologyLink{ID: "l1", SourceNodeID: "r1", SourcePortID: "Gi0/0", TargetNodeID: "r2", TargetPortID: "Gi0/0"})

	pc1 := mgr.AddHost("pc1")
	pc1.Configure("192.168.1.10", "255.255.255.0", "192.168.1.1", "00:11:00:00:00:01")
	pc2 := mgr.AddHost("pc2")
	pc2.Configure("192.168.2.10", "255.255.255.0", "192.168.2.1", "00:11:00:00:00:02")

	mgr.AddLink(network.TopologyLink{ID: "l2", SourceNodeID: "pc1", SourcePortID: "FastEthernet0", TargetNodeID: "r1", TargetPortID: "Gi0/1"})
	mgr.AddLink(network.TopologyLink{ID: "l3", SourceNodeID: "pc2", SourcePortID: "FastEthernet0", TargetNodeID: "r2", TargetPortID: "Gi0/1"})

	_ = mgr.AddStaticRoute("r1", "192.168.2.0/24", "10.0.0.2", "Gi0/0", 1)
	_ = mgr.AddStaticRoute("r2", "192.168.1.0/24", "10.0.0.1", "Gi0/0", 1)

	if err := mgr.StartPing("pc1", "192.168.2.10", "tier4"); err != nil {
		t.Fatalf("StartPing: %v", err)
	}

	for i := 0; i < 8000; i++ {
		event, ok := sched.Step()
		if ok {
			switch event.Type {
			case engine.EventPacketRx, engine.EventTimerICMP:
				mgr.HandleSimulationEvent(event)
			}
		}
		for _, r := range mgr.DrainPingResults() {
			if r.Success {
				events := mgr.GetEventLog(20, network.EventPacketTX)
				if len(events) == 0 {
					t.Fatal("expected packet TX events in log")
				}
				return
			}
			t.Fatalf("ping failed: %s", r.Message)
		}
	}
	t.Fatal("ping did not complete")
}
