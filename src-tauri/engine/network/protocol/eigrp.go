package protocol

import (
	"net"
	"sync"
	"time"

	"netforge/engine/pdu"
)

const (
	RouteEIGRP RouteProtocol = "D"
	AdminDistEIGRP = 90
)

// EigrpRoute is an EIGRP topology table entry.
type EigrpRoute struct {
	Network   string
	NextHop   pdu.IPAddress
	Interface string
	Metric    int
	Feasible  bool
	Learned   time.Duration
}

// EigrpDaemon runs simplified EIGRP on a router.
type EigrpDaemon struct {
	ASNumber  int
	RouterID  pdu.IPAddress
	Enabled   bool
	Networks  []string
	Routes    map[string]*EigrpRoute
	Neighbors map[pdu.IPAddress]bool
	mu        sync.RWMutex
}

// NewEigrpDaemon creates an EIGRP process.
func NewEigrpDaemon(routerID pdu.IPAddress, as int) *EigrpDaemon {
	return &EigrpDaemon{
		ASNumber: as, RouterID: routerID,
		Routes: make(map[string]*EigrpRoute),
		Neighbors: make(map[pdu.IPAddress]bool),
	}
}

func (e *EigrpDaemon) Enable() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Enabled = true
}

func (e *EigrpDaemon) AddNetwork(cidr string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Networks = append(e.Networks, cidr)
}

func (e *EigrpDaemon) AddNeighbor(ip pdu.IPAddress) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Neighbors[ip] = true
}

func (e *EigrpDaemon) ProcessUpdate(routes []EigrpRoute, sender pdu.IPAddress, inIface string, simTime time.Duration) []EigrpRoute {
	e.mu.Lock()
	defer e.mu.Unlock()

	updated := make([]EigrpRoute, 0)
	for _, route := range routes {
		newMetric := route.Metric + 100000
		existing, found := e.Routes[route.Network]
		if !found || newMetric < existing.Metric {
			r := EigrpRoute{
				Network: route.Network, NextHop: sender, Interface: inIface,
				Metric: newMetric, Feasible: true, Learned: simTime,
			}
			e.Routes[route.Network] = &r
			updated = append(updated, r)
		}
	}
	return updated
}

func (e *EigrpDaemon) GetAdvertisement(local []EigrpRoute) []EigrpRoute {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := append([]EigrpRoute{}, local...)
	for _, r := range e.Routes {
		out = append(out, *r)
	}
	return out
}

func (e *EigrpDaemon) MatchNetwork(ip pdu.IPAddress) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	ipAddr := net.ParseIP(string(ip))
	for _, cidr := range e.Networks {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ipNet.Contains(ipAddr) {
			return true
		}
	}
	return false
}
