package network

import (
	"fmt"
	"time"

	"netforge/engine"
	"netforge/engine/pdu"
)

// TracerouteHop is one hop in a traceroute result.
type TracerouteHop struct {
	Hop     int     `json:"hop"`
	Address string  `json:"address"`
	RTT     float64 `json:"rtt_ms"`
	Status  string  `json:"status"`
}

// TracerouteResult is the full traceroute output.
type TracerouteResult struct {
	SourceID  string          `json:"source_id"`
	DestIP    string          `json:"dest_ip"`
	Hops      []TracerouteHop `json:"hops"`
	Success   bool            `json:"success"`
	RequestID string          `json:"request_id,omitempty"`
}

// StartTraceroute runs TTL-incrementing probes from a host or router.
func (m *Manager) StartTraceroute(sourceID, destIP, requestID string) ([]TracerouteHop, error) {
	var startRouter string
	if host, ok := m.GetHost(sourceID); ok {
		startRouter = host.UplinkNode
	} else if r, ok := m.GetRouter(sourceID); ok {
		startRouter = r.ID
	} else {
		return nil, fmt.Errorf("device %s not found", sourceID)
	}

	simTime := m.SimNow()
	hops := make([]TracerouteHop, 0, 30)
	dest := pdu.IPAddress(destIP)

	for ttl := 1; ttl <= 30; ttl++ {
		replyFrom, rtt, reached := m.probeTTL(startRouter, dest, ttl, simTime)
		hop := TracerouteHop{Hop: ttl, RTT: rtt}
		if replyFrom != "" {
			hop.Address = string(replyFrom)
			hop.Status = "ok"
		} else {
			hop.Status = "*"
		}
		hops = append(hops, hop)
		if reached {
			return hops, nil
		}
		simTime += time.Millisecond
	}
	return hops, nil
}

func (m *Manager) probeTTL(startRouter string, dest pdu.IPAddress, ttl int, simTime time.Duration) (pdu.IPAddress, float64, bool) {
	currentNode := startRouter
	for hop := 1; hop <= ttl; hop++ {
		router, ok := m.GetRouter(currentNode)
		if !ok {
			break
		}
		route, found := router.MatchRoute(dest)
		if !found {
			break
		}
		if hop == ttl {
			// Time exceeded: responder is current router interface IP
			ifaceIP := router.Interfaces[route.Interface]
			if dest == ifaceIP || router.OwnsIP(dest) {
				return dest, float64(hop), true
			}
			return ifaceIP, float64(hop) * 2.0, false
		}
		// find next router via topology
		nextRouter, _, found := m.FindNeighborOnPort(currentNode, route.Interface)
		if !found {
			break
		}
		currentNode = nextRouter
	}
	_ = engine.EventTimerICMP
	return "", 0, false
}
