package protocol

import (
	"net"
	"sync"
	"time"

	"netforge/engine/pdu"
)

const (
	RipMaxHop      = 15
	RipInfinity    = 16
	RipUpdateTimer = 30 * time.Second
	RipPort        = 520
)

// RipRoute is a single RIP routing table entry in the distance-vector database.
type RipRoute struct {
	Network   string
	NextHop   pdu.IPAddress
	Interface string
	Metric    int
	Learned   time.Duration
}

// RipUpdate is a RIP response PDU containing route advertisements.
type RipUpdate struct {
	SenderRouterID string
	Routes         []RipRouteEntry
}

// RipRouteEntry is one route inside a RIP update packet.
type RipRouteEntry struct {
	Network string
	Metric  int
}

// RipDaemon runs the RIP distance-vector protocol on a virtual router.
type RipDaemon struct {
	RouterID  pdu.IPAddress
	Enabled   bool
	Networks  []string
	Routes    map[string]*RipRoute // keyed by network CIDR
	Neighbors map[string]pdu.IPAddress // portID -> neighbor router IP
	mu        sync.RWMutex
}

// NewRipDaemon creates a new RIP process instance.
func NewRipDaemon(routerID pdu.IPAddress) *RipDaemon {
	return &RipDaemon{
		RouterID:  routerID,
		Routes:    make(map[string]*RipRoute),
		Neighbors: make(map[string]pdu.IPAddress),
	}
}

// Enable activates the RIP process.
func (r *RipDaemon) Enable() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Enabled = true
}

// AddNetwork registers a RIP network statement.
func (r *RipDaemon) AddNetwork(cidr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Networks = append(r.Networks, cidr)
}

// MatchInterface returns true if the interface IP matches a configured RIP network.
func (r *RipDaemon) MatchInterface(ip pdu.IPAddress) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ipAddr := net.ParseIP(string(ip))
	if ipAddr == nil {
		return false
	}

	for _, cidr := range r.Networks {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			_, ipNet, err = net.ParseCIDR(cidr + "/24")
			if err != nil {
				continue
			}
		}
		if ipNet.Contains(ipAddr) {
			return true
		}
	}
	return false
}

// AddNeighbor records a directly connected RIP peer on a port.
func (r *RipDaemon) AddNeighbor(portID string, neighborIP pdu.IPAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Neighbors[portID] = neighborIP
}

// BuildUpdate assembles a RIP response from the current routing table.
func (r *RipDaemon) BuildUpdate(localRoutes []RipRoute) *RipUpdate {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]RipRouteEntry, 0, len(localRoutes))
	for _, route := range localRoutes {
		entries = append(entries, RipRouteEntry{
			Network: route.Network,
			Metric:  route.Metric,
		})
	}

	return &RipUpdate{
		SenderRouterID: string(r.RouterID),
		Routes:         entries,
	}
}

// ProcessUpdate applies Bellman-Ford distance-vector logic to an incoming RIP update.
func (r *RipDaemon) ProcessUpdate(update *RipUpdate, senderIP pdu.IPAddress, inInterface string, simTime time.Duration) []RipRoute {
	r.mu.Lock()
	defer r.mu.Unlock()

	updated := make([]RipRoute, 0)

	for _, entry := range update.Routes {
		newMetric := entry.Metric + 1
		if newMetric >= RipInfinity {
			continue
		}

		existing, found := r.Routes[entry.Network]
		if !found || newMetric < existing.Metric {
			route := RipRoute{
				Network:   entry.Network,
				NextHop:   senderIP,
				Interface: inInterface,
				Metric:    newMetric,
				Learned:   simTime,
			}
			r.Routes[entry.Network] = &route
			updated = append(updated, route)
		}
	}

	return updated
}

// GetRoutes returns a snapshot of the RIP routing database.
func (r *RipDaemon) GetRoutes() []RipRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]RipRoute, 0, len(r.Routes))
	for _, route := range r.Routes {
		copied := *route
		result = append(result, copied)
	}
	return result
}

// RemoveStaleRoutes purges routes learned from a dead neighbor.
func (r *RipDaemon) RemoveStaleRoutes(neighborIP pdu.IPAddress) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for cidr, route := range r.Routes {
		if route.NextHop == neighborIP {
			delete(r.Routes, cidr)
		}
	}
}
