package network_test

import (
	"testing"

	"netforge/engine/network"
	"netforge/engine/pdu"
)

func TestQoSPolicyMap(t *testing.T) {
	rtr := network.NewRouter("r1")
	rtr.AddClassMap(network.QoSClassMap{Name: "voice", MatchType: network.MatchProtocol, MatchVal: "sip"})
	rtr.AddPolicyMap(network.QoSPolicyMap{
		Name: "VOICE-POLICY",
		Classes: []network.QoSPolicyClass{{
			ClassMap: "voice",
			Actions:  []network.QoSPolicyAction{{Type: "priority", Value: 70, Unit: "percent"}},
		}},
	})
	rtr.ApplyServicePolicy("Gi0/0", "VOICE-POLICY", "output")
	rtr.AddInterface("Gi0/0", "10.0.0.1", "255.255.255.252", "00:00:00:00:00:01")

	pkt := &pdu.IPv4Packet{Protocol: pdu.ProtoUDP, SourceIP: "10.0.0.1", DestinationIP: "10.0.0.2"}
	dscp, drop := rtr.ApplyQoSToPacket("Gi0/0", pkt, 0)
	if drop {
		t.Fatal("unexpected police drop")
	}
	if dscp != pdu.DSCPEF {
		t.Fatalf("expected EF DSCP for voice, got %d", dscp)
	}
}

func TestSCCPRegistration(t *testing.T) {
	mgr := network.NewManager()
	phone := mgr.AddVoIPPhone("phone1")
	cm := mgr.AddCallManager("cucm1")
	cm.IP = "192.168.1.50"
	if err := mgr.SendSCCPRegister("phone1", "cucm1"); err != nil {
		t.Fatal(err)
	}
	if !phone.Registered {
		t.Fatal("expected SCCP registration")
	}
}

func TestSIPCallSignaling(t *testing.T) {
	mgr := network.NewManager()
	phone := mgr.AddVoIPPhone("phone1")
	phone.Configure("1001", "192.168.150.10", "192.168.10.10", "192.168.1.50", 150)
	phone.Registered = true
	mgr.AddCallManager("cucm1")

	call, err := mgr.InitiateSIPCall("phone1", "1002")
	if err != nil {
		t.Fatal(err)
	}
	if call.State != network.CallRinging {
		t.Fatalf("expected ringing, got %s", call.State)
	}
}

func Test5GNRAttach(t *testing.T) {
	mgr := network.NewManager()
	gw := mgr.AddCellularGateway("lte1")
	gw.Configure5GNR("n78", 620000, 30, 100)
	gw.Attach5GCore("001-01", "1-000001", "internet")
	ue := mgr.AddMobileUE("ue1")
	if err := mgr.Attach5GNRUE("ue1", "lte1", "10.99.0.5", "n78", 620000); err != nil {
		t.Fatal(err)
	}
	if ue.Technology != "5G-NR" || ue.RAT != network.RA5GN {
		t.Fatal("expected 5G NR attach")
	}
	status := gw.Format5GStatus()
	if len(status) < 2 {
		t.Fatal("expected 5G status output")
	}
}

func TestSwitchMLSQoS(t *testing.T) {
	sw := network.NewSwitch("sw1")
	sw.RegisterPort("Fa0/1")
	sw.ApplySwitchQoSPolicy("Fa0/1", network.SwitchQoSPolicy{TrustDSCP: true})
	prio := sw.ClassifySwitchFrame("Fa0/1", pdu.DSCPEF)
	if prio != 1 {
		t.Fatal("expected priority queue for EF traffic")
	}
}
