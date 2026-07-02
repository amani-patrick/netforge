package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"netforge/engine"
	"netforge/engine/cli"
	"netforge/engine/network"
	"netforge/engine/network/protocol"
	"netforge/engine/pdu"

	"github.com/gorilla/websocket"
)

// UIEvent defines incoming payload schemas from the React interface.
type UIEvent struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

var (
	scheduler   = engine.NewScheduler()
	netMgr      = network.NewManager()
	cliSessions = cli.NewSessionStore()
	upgrader    = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	clients   = make(map[*websocket.Conn]bool)
	clientsMu sync.Mutex
)

func main() {
	fmt.Println("NetForge Go Core Simulation Engine Starting Up...")

	netMgr.SetScheduler(scheduler)
	netMgr.ScheduleProtocolTimers(scheduler)
	go runSimulationLoop()

	http.HandleFunc("/ws", handleIPCConnection)

	port := 8085
	log.Printf("Local WebSocket Pipeline listening on ws://127.0.0.1:%d/ws", port)
	if err := http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", port), nil); err != nil {
		log.Fatalf("Fatal Error starting WebSocket Server: %v", err)
	}
}

func runSimulationLoop() {
	for {
		event, active := scheduler.Step()
		if active {
			switch event.Type {
			case engine.EventTimerOSPF, engine.EventTimerRIP,
				engine.EventTimerEIGRP, engine.EventTimerBGP,
				engine.EventTimerCDP, engine.EventTimerSTP,
				engine.EventTimerHSRP, engine.EventTimerVTP:
				netMgr.HandleTimerEvent(event.Type, scheduler)
			case engine.EventPacketRx, engine.EventTimerICMP:
				netMgr.HandleSimulationEvent(event)
			}

			for _, result := range netMgr.DrainPingResults() {
				broadcastToUI(map[string]interface{}{
					"event": "PING_RESULT",
					"data":  result,
				})
			}

			broadcastToUI(map[string]interface{}{
				"event": "SIM_TICK",
				"data":  event,
			})
		}
		time.Sleep(1 * time.Millisecond)
	}
}

func handleIPCConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	clientsMu.Lock()
	clients[conn] = true
	clientsMu.Unlock()

	log.Println("React Webview UI hooked successfully into Go Pipeline.")

	defer func() {
		clientsMu.Lock()
		delete(clients, conn)
		clientsMu.Unlock()
		conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading client instruction payload: %v", err)
			break
		}

		var event UIEvent
		if err := json.Unmarshal(msg, &event); err != nil {
			continue
		}

		handleUICommand(event, conn)
	}
}

func handleUICommand(event UIEvent, conn *websocket.Conn) {
	switch event.Type {
	case "ADD_ROUTER":
		var payload struct {
			ID         string `json:"id"`
			Interfaces []struct {
				PortID string `json:"port_id"`
				IP     string `json:"ip"`
				Mask   string `json:"mask"`
				MAC    string `json:"mac"`
			} `json:"interfaces"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router := netMgr.AddRouter(payload.ID)
			for _, iface := range payload.Interfaces {
				router.AddInterface(iface.PortID, pdu.IPAddress(iface.IP), iface.Mask, pdu.MACAddress(iface.MAC))
			}
			log.Printf("Router [%s] provisioned with %d interfaces.", payload.ID, len(payload.Interfaces))
		}

	case "ADD_SWITCH":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.AddSwitch(payload.ID)
			log.Printf("Switch [%s] provisioned.", payload.ID)
		}

	case "ADD_HOST":
		var payload struct {
			ID      string `json:"id"`
			IP      string `json:"ip"`
			Mask    string `json:"mask"`
			Gateway string `json:"gateway"`
			MAC     string `json:"mac"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			host := netMgr.AddHost(payload.ID)
			host.Configure(pdu.IPAddress(payload.IP), payload.Mask, pdu.IPAddress(payload.Gateway), pdu.MACAddress(payload.MAC))
			log.Printf("Host [%s] provisioned at %s", payload.ID, payload.IP)
		}

	case "CONNECT_LINK":
		var payload struct {
			ID           string  `json:"id"`
			SourceNodeID string  `json:"source_node_id"`
			SourcePortID string  `json:"source_port_id"`
			TargetNodeID string  `json:"target_node_id"`
			TargetPortID string  `json:"target_port_id"`
			CableLength  float64 `json:"cable_length"`
			Bandwidth    int64   `json:"bandwidth"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.AddLink(network.TopologyLink{
				ID:           payload.ID,
				SourceNodeID: payload.SourceNodeID,
				SourcePortID: payload.SourcePortID,
				TargetNodeID: payload.TargetNodeID,
				TargetPortID: payload.TargetPortID,
				CableLength:  payload.CableLength,
				Bandwidth:    payload.Bandwidth,
			})
			log.Printf("Link connected: %s:%s <-> %s:%s", payload.SourceNodeID, payload.SourcePortID, payload.TargetNodeID, payload.TargetPortID)
		}

	case "REMOVE_DEVICE":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if err := netMgr.RemoveDevice(payload.ID); err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "DEVICE_REMOVED", "data": map[string]string{"id": payload.ID}})
		}

	case "REMOVE_LINK":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if err := netMgr.RemoveLink(payload.ID); err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "LINK_REMOVED", "data": map[string]string{"id": payload.ID}})
		}

	case "ADD_STATIC_ROUTE":
		var payload struct {
			RouterID  string `json:"router_id"`
			CIDR      string `json:"cidr"`
			NextHop   string `json:"next_hop"`
			Interface string `json:"interface"`
			Metric    int    `json:"metric"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			metric := payload.Metric
			if metric == 0 {
				metric = 1
			}
			err := netMgr.AddStaticRoute(payload.RouterID, payload.CIDR, payload.NextHop, payload.Interface, metric)
			if err != nil {
				sendToClient(conn, map[string]interface{}{
					"event": "ERROR",
					"data":  map[string]string{"message": err.Error()},
				})
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "STATIC_ROUTE_ADDED",
				"data":  map[string]string{"router_id": payload.RouterID, "cidr": payload.CIDR},
			})
		}

	case "CONFIG_OSPF":
		var payload struct {
			RouterID  string `json:"router_id"`
			ProcessID int    `json:"process_id"`
			Networks  []struct {
				CIDR string `json:"cidr"`
				Area int    `json:"area"`
			} `json:"networks"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if !ok {
				return
			}
			router.EnableOspf(payload.ProcessID)
			networks := make([]protocol.OspfNetwork, 0, len(payload.Networks))
			for _, n := range payload.Networks {
				networks = append(networks, protocol.OspfNetwork{CIDR: n.CIDR, Area: n.Area})
			}
			router.ConfigureOspfNetworks(networks)
			netMgr.RunOspfHelloCycle()
			sendToClient(conn, map[string]interface{}{
				"event": "OSPF_CONFIGURED",
				"data":  map[string]string{"router_id": payload.RouterID},
			})
		}

	case "CONFIG_RIP":
		var payload struct {
			RouterID string   `json:"router_id"`
			Networks []string `json:"networks"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if !ok {
				return
			}
			router.EnableRip()
			router.ConfigureRipNetworks(payload.Networks)
			netMgr.RefreshRipNeighbors()
			netMgr.RunRipUpdateCycle()
			sendToClient(conn, map[string]interface{}{
				"event": "RIP_CONFIGURED",
				"data":  map[string]string{"router_id": payload.RouterID},
			})
		}

	case "SHOW_IP_ROUTE":
		var payload struct {
			RouterID string `json:"router_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			routes, err := netMgr.GetRouteTable(payload.RouterID)
			if err != nil {
				sendToClient(conn, map[string]interface{}{
					"event": "ROUTE_TABLE",
					"data":  map[string]string{"error": err.Error()},
				})
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "ROUTE_TABLE",
				"data": map[string]interface{}{
					"router_id": payload.RouterID,
					"routes":    routes,
				},
			})
		}

	case "SHOW_OSPF_NEIGHBOR":
		var payload struct {
			RouterID string `json:"router_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			neighbors, err := netMgr.GetOspfNeighbors(payload.RouterID)
			if err != nil {
				sendToClient(conn, map[string]interface{}{
					"event": "OSPF_NEIGHBORS",
					"data":  map[string]string{"error": err.Error()},
				})
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "OSPF_NEIGHBORS",
				"data": map[string]interface{}{
					"router_id": payload.RouterID,
					"neighbors": neighbors,
				},
			})
		}

	case "TRIGGER_PING":
		var payload struct {
			SourceID  string `json:"src_id"`
			DestIP    string `json:"dst_ip"`
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if err := netMgr.StartPing(payload.SourceID, payload.DestIP, payload.RequestID); err != nil {
				sendToClient(conn, map[string]interface{}{
					"event": "PING_RESULT",
					"data": map[string]interface{}{
						"source_id":  payload.SourceID,
						"dest_ip":    payload.DestIP,
						"success":    false,
						"message":    err.Error(),
						"request_id": payload.RequestID,
					},
				})
			}
		}

	case "SAVE_TOPOLOGY":
		var payload struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			path := payload.Path
			if path == "" {
				path = "topology.json"
			}
			if err := netMgr.SaveTopology(path); err != nil {
				sendToClient(conn, map[string]interface{}{
					"event": "ERROR",
					"data":  map[string]string{"message": err.Error()},
				})
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "TOPOLOGY_SAVED",
				"data":  map[string]string{"path": path},
			})
		}

	case "LOAD_TOPOLOGY":
		var payload struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			path := payload.Path
			if path == "" {
				path = "topology.json"
			}
			if err := netMgr.LoadTopology(path); err != nil {
				sendToClient(conn, map[string]interface{}{
					"event": "ERROR",
					"data":  map[string]string{"message": err.Error()},
				})
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "TOPOLOGY_LOADED",
				"data":  map[string]string{"path": path},
			})
		}

	case "EXPORT_TOPOLOGY":
		data, err := netMgr.SnapshotJSON()
		if err != nil {
			sendToClient(conn, map[string]interface{}{
				"event": "ERROR",
				"data":  map[string]string{"message": err.Error()},
			})
			return
		}
		sendToClient(conn, map[string]interface{}{
			"event": "TOPOLOGY_EXPORT",
			"data":  json.RawMessage(data),
		})

	case "CONFIG_VLAN_ACCESS":
		var payload struct {
			SwitchID string `json:"switch_id"`
			PortID   string `json:"port_id"`
			VLAN     int    `json:"vlan"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			_ = netMgr.ConfigureSwitchVLAN(payload.SwitchID, payload.PortID, payload.VLAN)
		}

	case "CONFIG_VLAN_TRUNK":
		var payload struct {
			SwitchID string `json:"switch_id"`
			PortID   string `json:"port_id"`
			Native   int    `json:"native"`
			Allowed  []int  `json:"allowed"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			_ = netMgr.ConfigureSwitchTrunk(payload.SwitchID, payload.PortID, payload.Native, payload.Allowed)
		}

	case "CONFIG_SUBINTERFACE":
		var payload struct {
			RouterID   string `json:"router_id"`
			ParentPort string `json:"parent_port"`
			VLAN       int    `json:"vlan"`
			IP         string `json:"ip"`
			Mask       string `json:"mask"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			subID, err := netMgr.ConfigureRouterSubInterface(payload.RouterID, payload.ParentPort, payload.VLAN, payload.IP, payload.Mask)
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "SUBINTERFACE_CREATED", "data": map[string]string{"subinterface": subID}})
		}

	case "CONFIG_ACL":
		var payload struct {
			RouterID string `json:"router_id"`
			ACLName  string `json:"acl_name"`
			Action   string `json:"action"`
			Protocol string `json:"protocol"`
			SrcNet   string `json:"src_net"`
			DstNet   string `json:"dst_net"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.AddACLRule(payload.ACLName, network.ACLRule{
					Action: network.ACLAction(payload.Action), Protocol: payload.Protocol,
					SrcNet: payload.SrcNet, DstNet: payload.DstNet,
				})
			}
		}

	case "APPLY_ACL":
		var payload struct {
			RouterID string `json:"router_id"`
			PortID   string `json:"port_id"`
			ACLName  string `json:"acl_name"`
			Direction string `json:"direction"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				pol := network.IfacePolicy{Up: true}
				if payload.Direction == "in" {
					pol.InboundACL = payload.ACLName
				} else {
					pol.OutboundACL = payload.ACLName
				}
				router.SetIfacePolicy(payload.PortID, pol)
			}
		}

	case "CONFIG_NAT_STATIC":
		var payload struct {
			RouterID     string `json:"router_id"`
			InsideLocal  string `json:"inside_local"`
			InsideGlobal string `json:"inside_global"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.AddStaticNAT(pdu.IPAddress(payload.InsideLocal), pdu.IPAddress(payload.InsideGlobal))
			}
		}

	case "CONFIG_NAT_OVERLOAD":
		var payload struct {
			RouterID string `json:"router_id"`
			PortID   string `json:"port_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.EnableNATOverload(payload.PortID)
			}
		}

	case "CONFIG_NAT_INSIDE":
		var payload struct {
			RouterID string `json:"router_id"`
			PortID   string `json:"port_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.MarkNATInside(payload.PortID)
			}
		}

	case "CONFIG_DHCP_POOL":
		var payload struct {
			RouterID      string `json:"router_id"`
			Name          string `json:"name"`
			Network       string `json:"network"`
			DefaultRouter string `json:"default_router"`
			DNSServer     string `json:"dns_server"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.AddDHCPPool(network.DHCPPool{
					Name: payload.Name, Network: payload.Network,
					DefaultRouter: pdu.IPAddress(payload.DefaultRouter),
					DNSServer: pdu.IPAddress(payload.DNSServer),
				})
			}
		}

	case "HOST_DHCP":
		var payload struct {
			HostID string `json:"host_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			ip, gw, dns, mask, err := netMgr.HostDHCPRequest(payload.HostID)
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "DHCP_ASSIGNED",
				"data": map[string]string{
					"host_id": payload.HostID, "ip": string(ip),
					"gateway": string(gw), "dns": string(dns), "mask": mask,
				},
			})
		}

	case "CONFIG_DNS":
		var payload struct {
			RouterID string `json:"router_id"`
			Name     string `json:"name"`
			IP       string `json:"ip"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.AddDNSRecord(payload.Name, pdu.IPAddress(payload.IP))
			}
		}

	case "DNS_LOOKUP":
		var payload struct {
			RouterID string `json:"router_id"`
			Name     string `json:"name"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if !ok {
				return
			}
			ip, found := router.LookupDNS(payload.Name)
			sendToClient(conn, map[string]interface{}{
				"event": "DNS_RESULT",
				"data": map[string]interface{}{"name": payload.Name, "ip": string(ip), "found": found},
			})
		}

	case "CONFIG_EIGRP":
		var payload struct {
			RouterID string   `json:"router_id"`
			ASNumber int      `json:"as_number"`
			Networks []string `json:"networks"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.EnableEigrp(payload.ASNumber)
				router.ConfigureEigrpNetworks(payload.Networks)
				netMgr.RunEigrpUpdateCycle()
			}
		}

	case "CONFIG_BGP":
		var payload struct {
			RouterID string `json:"router_id"`
			LocalAS  int    `json:"local_as"`
			PeerIP   string `json:"peer_ip"`
			RemoteAS int    `json:"remote_as"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.EnableBgp(payload.LocalAS)
				router.AddBgpPeer(pdu.IPAddress(payload.PeerIP), payload.RemoteAS)
				netMgr.RunBgpUpdateCycle()
			}
		}

	case "INTERFACE_SHUTDOWN":
		var payload struct {
			RouterID string `json:"router_id"`
			PortID   string `json:"port_id"`
			Shutdown bool   `json:"shutdown"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.SetInterfaceShutdown(payload.PortID, payload.Shutdown)
			}
		}

	case "SHOW_CDP_NEIGHBORS":
		var payload struct {
			RouterID string `json:"router_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if !ok {
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "CDP_NEIGHBORS",
				"data":  map[string]interface{}{"router_id": payload.RouterID, "neighbors": router.CDPNeighbors},
			})
		}

	case "ADD_AP":
		var payload struct {
			ID       string `json:"id"`
			SSID     string `json:"ssid"`
			Security string `json:"security"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			ap := netMgr.AddAccessPoint(payload.ID)
			ap.Configure(payload.SSID, payload.Security, payload.Password)
			sendToClient(conn, map[string]interface{}{"event": "AP_ADDED", "data": map[string]string{"id": payload.ID}})
		}

	case "ADD_ASA":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.AddASAFirewall(payload.ID)
			sendToClient(conn, map[string]interface{}{"event": "ASA_ADDED", "data": map[string]string{"id": payload.ID}})
		}

	case "CONFIG_ASA_INTERFACE":
		var payload struct {
			ASAID  string `json:"asa_id"`
			PortID string `json:"port_id"`
			IP     string `json:"ip"`
			Mask   string `json:"mask"`
			MAC    string `json:"mac"`
			Zone   string `json:"zone"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			asa, ok := netMgr.GetASAFirewall(payload.ASAID)
			if ok {
				asa.AddInterface(payload.PortID, pdu.IPAddress(payload.IP), payload.Mask, pdu.MACAddress(payload.MAC), network.SecurityZone(payload.Zone))
			}
		}

	case "CONFIG_WAN_SERIAL":
		var payload struct {
			RouterID  string `json:"router_id"`
			PortID    string `json:"port_id"`
			Encap     string `json:"encap"`
			Bandwidth int64  `json:"bandwidth"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.ConfigureWANSerial(payload.RouterID, payload.PortID, payload.Encap, payload.Bandwidth)
		}

	case "CONFIG_FRAME_RELAY":
		var payload struct {
			ID           string `json:"id"`
			SourceNodeID string `json:"source_node_id"`
			SourcePortID string `json:"source_port_id"`
			SourceDLCI   uint16 `json:"source_dlci"`
			TargetNodeID string `json:"target_node_id"`
			TargetPortID string `json:"target_port_id"`
			TargetDLCI   uint16 `json:"target_dlci"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.ConfigureFrameRelayMap(network.WANLink{
				ID: payload.ID, SourceNodeID: payload.SourceNodeID, SourcePortID: payload.SourcePortID,
				SourceDLCI: payload.SourceDLCI, TargetNodeID: payload.TargetNodeID,
				TargetPortID: payload.TargetPortID, TargetDLCI: payload.TargetDLCI, Encap: "frame-relay",
			})
		}

	case "WIRELESS_ASSOCIATE":
		var payload struct {
			APID       string `json:"ap_id"`
			ClientMAC  string `json:"client_mac"`
			Password   string `json:"password"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			err := netMgr.WirelessAssociate(payload.APID, pdu.MACAddress(payload.ClientMAC), payload.Password)
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "WIRELESS_ASSOCIATED", "data": map[string]string{"ap_id": payload.APID}})
		}

	case "HOST_IPV6":
		var payload struct {
			HostID  string `json:"host_id"`
			IPv6    string `json:"ipv6"`
			Gateway string `json:"gateway"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			host, ok := netMgr.GetHost(payload.HostID)
			if ok {
				host.ConfigureIPv6(pdu.IPv6Address(payload.IPv6), pdu.IPv6Address(payload.Gateway))
			}
		}

	case "CONFIG_IPV6_INTERFACE":
		var payload struct {
			RouterID   string `json:"router_id"`
			PortID     string `json:"port_id"`
			IPv6       string `json:"ipv6"`
			PrefixLen  int    `json:"prefix_len"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				router.AddIPv6Interface(payload.PortID, pdu.IPv6Address(payload.IPv6), payload.PrefixLen)
			}
		}

	case "SHOW_IPV6_ROUTE":
		var payload struct {
			RouterID string `json:"router_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if !ok {
				return
			}
			sendToClient(conn, map[string]interface{}{
				"event": "IPV6_ROUTE_TABLE",
				"data":  map[string]interface{}{"router_id": payload.RouterID, "routes": router.FormatIPv6RouteTable()},
			})
		}

	case "EXEC_CLI":
		var payload struct {
			DeviceID string `json:"device_id"`
			Command  string `json:"command"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			exec := &cli.ExecutorWithSession{Mgr: netMgr, Sessions: cliSessions}
			result := exec.Execute(payload.DeviceID, payload.Command)
			sendToClient(conn, map[string]interface{}{
				"event": "CLI_RESULT",
				"data":  result,
			})
		}

	case "GET_EVENT_LOG":
		var payload struct {
			Limit    int    `json:"limit"`
			Category string `json:"category"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			sendToClient(conn, map[string]interface{}{
				"event": "EVENT_LOG",
				"data":  netMgr.GetEventLog(payload.Limit, network.EventCategory(payload.Category)),
			})
		}

	case "CLEAR_EVENT_LOG":
		netMgr.ClearEventLog()

	case "GET_PORT_CAPTURE":
		var payload struct {
			NodeID string `json:"node_id"`
			PortID string `json:"port_id"`
			Limit  int    `json:"limit"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			sendToClient(conn, map[string]interface{}{
				"event": "PORT_CAPTURE",
				"data":  netMgr.GetPortCapture(payload.NodeID, payload.PortID, payload.Limit),
			})
		}

	case "LIST_DEVICES":
		devices := netMgr.ListDevices()
		rows := make([]map[string]interface{}, 0, len(devices))
		for _, d := range devices {
			rows = append(rows, map[string]interface{}{
				"id": d.DeviceID(), "type": string(d.DeviceKind()), "ports": d.PortIDs(),
			})
		}
		sendToClient(conn, map[string]interface{}{"event": "DEVICE_LIST", "data": rows})

	case "ADD_ACTIVITY_GOAL":
		var payload struct {
			ID          string            `json:"id"`
			Type        string            `json:"type"`
			Description string            `json:"description"`
			Params      map[string]string `json:"params"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.AddActivityGoal(network.ActivityGoal{
				ID: payload.ID, Type: network.GoalType(payload.Type),
				Description: payload.Description, Params: payload.Params,
			})
		}

	case "EVALUATE_ACTIVITY":
		results := netMgr.EvaluateActivity()
		sendToClient(conn, map[string]interface{}{"event": "ACTIVITY_RESULTS", "data": results})

	case "WRITE_MEMORY":
		var payload struct {
			DeviceID string `json:"device_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.WriteMemory(payload.DeviceID)
			sendToClient(conn, map[string]interface{}{"event": "CONFIG_SAVED", "data": map[string]string{"device_id": payload.DeviceID}})
		}

	case "CONFIG_HSRP":
		var payload struct {
			RouterID  string `json:"router_id"`
			PortID    string `json:"port_id"`
			GroupID   int    `json:"group_id"`
			VirtualIP string `json:"virtual_ip"`
			Priority  int    `json:"priority"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			_ = netMgr.ConfigureHSRPOnRouter(payload.RouterID, payload.PortID, payload.GroupID, payload.VirtualIP, payload.Priority)
		}

	case "CONFIG_VTP":
		var payload struct {
			SwitchID string `json:"switch_id"`
			Domain   string `json:"domain"`
			Mode     string `json:"mode"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			_ = netMgr.ConfigureVTPOnSwitch(payload.SwitchID, payload.Domain, network.VTPMode(payload.Mode))
		}

	case "CONFIG_VOICE_VLAN":
		var payload struct {
			SwitchID  string `json:"switch_id"`
			PortID    string `json:"port_id"`
			DataVLAN  int    `json:"data_vlan"`
			VoiceVLAN int    `json:"voice_vlan"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			_ = netMgr.ConfigureVoiceVLAN(payload.SwitchID, payload.PortID, payload.DataVLAN, payload.VoiceVLAN)
		}

	case "ADD_VOIP_PHONE":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.AddVoIPPhone(payload.ID)
		}

	case "ADD_CALL_MANAGER":
		var payload struct {
			ID string `json:"id"`
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			cm := netMgr.AddCallManager(payload.ID)
			cm.IP = pdu.IPAddress(payload.IP)
		}

	case "ADD_CELLULAR_GATEWAY":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.AddCellularGateway(payload.ID)
		}

	case "ADD_MOBILE_UE":
		var payload struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			netMgr.AddMobileUE(payload.ID)
		}

	case "TRACEROUTE":
		var payload struct {
			SourceID string `json:"source_id"`
			DestIP   string `json:"dest_ip"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			hops, err := netMgr.StartTraceroute(payload.SourceID, payload.DestIP, "ipc")
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "TRACEROUTE_RESULT", "data": hops})
		}

	case "SHOW_HSRP":
		var payload struct {
			RouterID string `json:"router_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			router, ok := netMgr.GetRouter(payload.RouterID)
			if ok {
				sendToClient(conn, map[string]interface{}{"event": "HSRP_STATUS", "data": router.FormatHSRPStatus()})
			}
		}

	case "SHOW_VLAN":
		var payload struct {
			SwitchID string `json:"switch_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			sw, ok := netMgr.GetSwitch(payload.SwitchID)
			if ok {
				sendToClient(conn, map[string]interface{}{"event": "VLAN_TABLE", "data": sw.ListVLANs()})
			}
		}

	case "CONFIG_QOS_CLASS_MAP":
		var payload struct {
			RouterID string `json:"router_id"`
			Name     string `json:"name"`
			Match    string `json:"match"`
			Value    string `json:"value"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if r, ok := netMgr.GetRouter(payload.RouterID); ok {
				r.AddClassMap(network.QoSClassMap{Name: payload.Name, MatchType: network.MatchProtocol, MatchVal: payload.Value})
			}
		}

	case "CONFIG_QOS_POLICY_MAP":
		var payload struct {
			RouterID  string `json:"router_id"`
			Name      string `json:"name"`
			ClassName string `json:"class_name"`
			Action    string `json:"action"`
			Value     int    `json:"value"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if r, ok := netMgr.GetRouter(payload.RouterID); ok {
				r.AddPolicyMap(network.QoSPolicyMap{
					Name: payload.Name,
					Classes: []network.QoSPolicyClass{{
						ClassMap: payload.ClassName,
						Actions:  []network.QoSPolicyAction{{Type: payload.Action, Value: payload.Value, Unit: "percent"}},
					}},
				})
			}
		}

	case "SCCP_REGISTER":
		var payload struct {
			PhoneID string `json:"phone_id"`
			CMID    string `json:"cm_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			err := netMgr.SendSCCPRegister(payload.PhoneID, payload.CMID)
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "SCCP_REGISTERED", "data": payload})
		}

	case "SIP_CALL":
		var payload struct {
			PhoneID string `json:"phone_id"`
			Callee  string `json:"callee"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			call, err := netMgr.InitiateSIPCall(payload.PhoneID, payload.Callee)
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "SIP_CALL_STARTED", "data": call})
		}

	case "ATTACH_5G_NR":
		var payload struct {
			UEID      string `json:"ue_id"`
			GatewayID string `json:"gateway_id"`
			IP        string `json:"ip"`
			Band      string `json:"band"`
			ARFCN     int    `json:"arfcn"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			err := netMgr.Attach5GNRUE(payload.UEID, payload.GatewayID, pdu.IPAddress(payload.IP), payload.Band, payload.ARFCN)
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "ERROR", "data": map[string]string{"message": err.Error()}})
				return
			}
			sendToClient(conn, map[string]interface{}{"event": "5G_ATTACHED", "data": payload})
		}

	case "SHOW_CALLS":
		sendToClient(conn, map[string]interface{}{"event": "VOIP_CALLS", "data": netMgr.GetActiveCalls()})

	case "SHOW_QOS":
		var payload struct {
			RouterID string `json:"router_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if r, ok := netMgr.GetRouter(payload.RouterID); ok {
				sendToClient(conn, map[string]interface{}{"event": "QOS_POLICY", "data": r.FormatQoSPolicy()})
			}
		}

	case "SHOW_5G_STATUS":
		var payload struct {
			GatewayID string `json:"gateway_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if gw, ok := netMgr.GetCellularGateway(payload.GatewayID); ok {
				sendToClient(conn, map[string]interface{}{"event": "5G_STATUS", "data": gw.Format5GStatus()})
			}
		}

	case "NEGOTIATE_IKE":
		var payload struct {
			RouterID string `json:"router_id"`
			PeerIP   string `json:"peer_ip"`
			PSK      string `json:"psk"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			err := netMgr.NegotiateIKE(payload.RouterID, pdu.IPAddress(payload.PeerIP), payload.PSK)
			if err != nil {
				sendToClient(conn, map[string]interface{}{"event": "IKE_ERROR", "data": map[string]string{"error": err.Error()}})
				return
			}
			netMgr.InstallVPNRoutes(payload.RouterID)
			sendToClient(conn, map[string]interface{}{"event": "IKE_ESTABLISHED", "data": map[string]string{"router_id": payload.RouterID, "peer": payload.PeerIP}})
		}

	case "CONFIG_CRYPTO_MAP":
		var payload struct {
			RouterID     string `json:"router_id"`
			MapName      string `json:"map_name"`
			Seq          int    `json:"seq"`
			PeerIP       string `json:"peer_ip"`
			TransformSet string `json:"transform_set"`
			ACLName      string `json:"acl_name"`
			LocalSubnet  string `json:"local_subnet"`
			RemoteSubnet string `json:"remote_subnet"`
			Iface        string `json:"iface"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if r, ok := netMgr.GetRouter(payload.RouterID); ok {
				r.AddCryptoMapEntry(network.CryptoMapEntry{
					MapName: payload.MapName, Seq: payload.Seq,
					PeerIP: pdu.IPAddress(payload.PeerIP), TransformSet: payload.TransformSet,
					ACLName: payload.ACLName, LocalSubnet: payload.LocalSubnet, RemoteSubnet: payload.RemoteSubnet,
				})
				if payload.Iface != "" {
					r.ApplyCryptoMap(payload.Iface, payload.MapName)
				}
				sendToClient(conn, map[string]interface{}{"event": "CRYPTO_MAP_CONFIGURED", "data": payload})
			}
		}

	case "SHOW_CRYPTO_SA":
		var payload struct {
			RouterID string `json:"router_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			if r, ok := netMgr.GetRouter(payload.RouterID); ok {
				sendToClient(conn, map[string]interface{}{"event": "CRYPTO_SA", "data": r.FormatCryptoSA()})
			}
		}

	case "LOAD_VPN_LAB":
		data := netMgr.BuildVPNLab()
		sendToClient(conn, map[string]interface{}{"event": "VPN_LAB_LOADED", "data": data})
	}
}

func sendToClient(conn *websocket.Conn, data interface{}) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	_ = conn.WriteMessage(websocket.TextMessage, bytes)
}

func broadcastToUI(data interface{}) {
	clientsMu.Lock()
	defer clientsMu.Unlock()

	bytes, err := json.Marshal(data)
	if err != nil {
		return
	}

	for client := range clients {
		_ = client.WriteMessage(websocket.TextMessage, bytes)
	}
}
