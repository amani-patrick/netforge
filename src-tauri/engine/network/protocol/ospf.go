package protocol

import (
	"net"
	"sync"
	"time"

	"netforge/engine/pdu"
)

// OspfState represents standard OSPF neighbor adjacency states.
type OspfState string

const (
	OspfDown   OspfState = "DOWN"
	OspfInit   OspfState = "INIT"
	OspfTwoWay OspfState = "2-WAY"
	OspfFull   OspfState = "FULL"
)

const (
	OspfHelloInterval = 10 * time.Second
	OspfDeadInterval  = 40 * time.Second
	OspfMulticast     = pdu.IPAddress("224.0.0.5")
)

// LsaLink represents a directed connection from a router to a subnet or neighbor.
type LsaLink struct {
	ConnectedID string
	LinkType    string // "router" or "stub"
	Cost        int
}

// RouterLsa is a Type-1 Router LSA advertising local links.
type RouterLsa struct {
	RouterID string
	Sequence uint32
	Links    []LsaLink
	Age      time.Duration
}

// OspfHelloPacket is the OSPF Hello PDU payload.
type OspfHelloPacket struct {
	RouterID        pdu.IPAddress   `json:"router_id"`
	NetworkMask     string          `json:"network_mask"`
	HelloInterval   uint16          `json:"hello_interval"`
	DeadInterval    uint16          `json:"dead_interval"`
	ActiveNeighbors []pdu.IPAddress `json:"active_neighbors"`
}

// OspfNeighbor tracks adjacency state for a discovered neighbor.
type OspfNeighbor struct {
	RouterID  pdu.IPAddress
	State     OspfState
	LastSeen  time.Duration
	Interface string
}

// OspfNetwork defines an OSPF-enabled network statement.
type OspfNetwork struct {
	CIDR string
	Area int
}

// OspfDaemon runs inside each virtual router.
type OspfDaemon struct {
	RouterID   pdu.IPAddress
	ProcessID  int
	Enabled    bool
	Networks   []OspfNetwork
	Neighbors  map[pdu.IPAddress]*OspfNeighbor
	LSDB       map[string]RouterLsa
	Interfaces map[string]OspfIfaceConfig
	mu         sync.RWMutex
}

// OspfIfaceConfig holds per-interface OSPF parameters.
type OspfIfaceConfig struct {
	PortID string
	IP     pdu.IPAddress
	Mask   string
	Area   int
	Cost   int
}

// NewOspfDaemon initializes a fresh OSPF process instance.
func NewOspfDaemon(routerID pdu.IPAddress, processID int) *OspfDaemon {
	return &OspfDaemon{
		RouterID:   routerID,
		ProcessID:  processID,
		Neighbors:  make(map[pdu.IPAddress]*OspfNeighbor),
		LSDB:       make(map[string]RouterLsa),
		Interfaces: make(map[string]OspfIfaceConfig),
	}
}

// Enable activates the OSPF process.
func (o *OspfDaemon) Enable() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Enabled = true
}

// AddNetwork registers a network statement for interface matching.
func (o *OspfDaemon) AddNetwork(cidr string, area int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.Networks = append(o.Networks, OspfNetwork{CIDR: cidr, Area: area})
}

// MatchInterface returns true if the given interface IP falls within a configured network.
func (o *OspfDaemon) MatchInterface(ip pdu.IPAddress, mask string) (OspfNetwork, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	ipAddr := net.ParseIP(string(ip))
	if ipAddr == nil {
		return OspfNetwork{}, false
	}

	for _, netStmt := range o.Networks {
		_, ipNet, err := net.ParseCIDR(netStmt.CIDR)
		if err != nil {
			continue
		}
		if ipNet.Contains(ipAddr) {
			return netStmt, true
		}
	}
	return OspfNetwork{}, false
}

// EnableInterface marks an interface as OSPF-active.
func (o *OspfDaemon) EnableInterface(portID string, ip pdu.IPAddress, mask string, area int) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.Interfaces[portID] = OspfIfaceConfig{
		PortID: portID,
		IP:     ip,
		Mask:   mask,
		Area:   area,
		Cost:   1,
	}
}

// GenerateHello builds an outgoing Hello PDU for the given interface.
func (o *OspfDaemon) GenerateHello(ifaceName string, mask string) *OspfHelloPacket {
	o.mu.RLock()
	defer o.mu.RUnlock()

	neighborsList := make([]pdu.IPAddress, 0, len(o.Neighbors))
	for rID, n := range o.Neighbors {
		if n.State == OspfFull || n.State == OspfTwoWay {
			neighborsList = append(neighborsList, rID)
		}
	}

	return &OspfHelloPacket{
		RouterID:        o.RouterID,
		NetworkMask:     mask,
		HelloInterval:   10,
		DeadInterval:    40,
		ActiveNeighbors: neighborsList,
	}
}

// HandleIncomingHello processes a received Hello and returns the updated neighbor state.
func (o *OspfDaemon) HandleIncomingHello(incomingIface string, hello *OspfHelloPacket, simTime time.Duration) OspfState {
	o.mu.Lock()
	defer o.mu.Unlock()

	neighbor, exists := o.Neighbors[hello.RouterID]
	if !exists {
		o.Neighbors[hello.RouterID] = &OspfNeighbor{
			RouterID:  hello.RouterID,
			State:     OspfInit,
			LastSeen:  simTime,
			Interface: incomingIface,
		}
		return OspfInit
	}

	neighbor.LastSeen = simTime

	weAreSeen := false
	for _, seenID := range hello.ActiveNeighbors {
		if seenID == o.RouterID {
			weAreSeen = true
			break
		}
	}

	if weAreSeen {
		if neighbor.State == OspfInit {
			neighbor.State = OspfTwoWay
		}
		if neighbor.State == OspfTwoWay {
			neighbor.State = OspfFull
		}
	} else {
		neighbor.State = OspfInit
	}

	return neighbor.State
}

// CheckDeadInterval removes neighbors that have not sent Hellos within the dead interval.
func (o *OspfDaemon) CheckDeadInterval(simTime time.Duration) []pdu.IPAddress {
	o.mu.Lock()
	defer o.mu.Unlock()

	deadNeighbors := make([]pdu.IPAddress, 0)

	for id, neighbor := range o.Neighbors {
		if simTime-neighbor.LastSeen > OspfDeadInterval {
			deadNeighbors = append(deadNeighbors, id)
			delete(o.Neighbors, id)
		}
	}
	return deadNeighbors
}

// GenerateRouterLsa builds a Type-1 LSA from local interfaces and FULL adjacencies.
func (o *OspfDaemon) GenerateRouterLsa(connectedSubnets map[string]int) RouterLsa {
	o.mu.RLock()
	defer o.mu.RUnlock()

	links := make([]LsaLink, 0)

	for neighborID, neighbor := range o.Neighbors {
		if neighbor.State != OspfFull {
			continue
		}
		links = append(links, LsaLink{
			ConnectedID: string(neighborID),
			LinkType:    "router",
			Cost:        1,
		})
	}

	for subnet, cost := range connectedSubnets {
		links = append(links, LsaLink{
			ConnectedID: subnet,
			LinkType:    "stub",
			Cost:        cost,
		})
	}

	return RouterLsa{
		RouterID: string(o.RouterID),
		Sequence: 1,
		Links:    links,
	}
}

// UpdateLSDB merges a received LSA into the local link-state database.
func (o *OspfDaemon) UpdateLSDB(lsa RouterLsa) bool {
	o.mu.Lock()
	defer o.mu.Unlock()

	existing, found := o.LSDB[lsa.RouterID]
	if found && existing.Sequence >= lsa.Sequence {
		return false
	}
	o.LSDB[lsa.RouterID] = lsa
	return true
}

// GetLSDB returns a snapshot of the link-state database.
func (o *OspfDaemon) GetLSDB() map[string]RouterLsa {
	o.mu.RLock()
	defer o.mu.RUnlock()

	copy := make(map[string]RouterLsa, len(o.LSDB))
	for k, v := range o.LSDB {
		copy[k] = v
	}
	return copy
}

// GetNeighbors returns a snapshot of OSPF neighbor table.
func (o *OspfDaemon) GetNeighbors() []*OspfNeighbor {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]*OspfNeighbor, 0, len(o.Neighbors))
	for _, n := range o.Neighbors {
		copied := *n
		result = append(result, &copied)
	}
	return result
}

// SPFResult holds a computed OSPF route.
type SPFResult struct {
	Destination string
	NextHop     string
	Metric      int
}

// ComputeSPF runs Dijkstra over the LSDB and returns shortest-path results.
func (o *OspfDaemon) ComputeSPF(lsdb map[string]RouterLsa, rootID string) map[string]SPFResult {
	o.mu.RLock()
	defer o.mu.RUnlock()

	distances := make(map[string]int)
	nextHops := make(map[string]string)
	visited := make(map[string]bool)

	for nodeID := range lsdb {
		distances[nodeID] = 999999
	}
	distances[rootID] = 0

	for i := 0; i < len(lsdb); i++ {
		u := ""
		minDist := 999999
		for nodeID, dist := range distances {
			if !visited[nodeID] && dist < minDist {
				minDist = dist
				u = nodeID
			}
		}
		if u == "" {
			break
		}
		visited[u] = true

		lsa := lsdb[u]

		for _, link := range lsa.Links {
			if visited[link.ConnectedID] {
				continue
			}

			altCost := distances[u] + link.Cost
			if altCost < distances[link.ConnectedID] {
				distances[link.ConnectedID] = altCost
				if u == rootID {
					nextHops[link.ConnectedID] = link.ConnectedID
				} else {
					nextHops[link.ConnectedID] = nextHops[u]
				}
			}
		}
	}

	results := make(map[string]SPFResult)
	for dest, metric := range distances {
		if dest == rootID || metric >= 999999 {
			continue
		}
		results[dest] = SPFResult{
			Destination: dest,
			NextHop:     nextHops[dest],
			Metric:      metric,
		}
	}
	return results
}
