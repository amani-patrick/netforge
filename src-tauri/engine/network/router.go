package network

import (
	"net"

	"netforge/engine/network/protocol"
	"netforge/engine/pdu"
)

// RouteEntry represents a single row in the routing table.
type RouteEntry struct {
	Network   *net.IPNet
	NextHop   pdu.IPAddress
	Interface string
	Metric    int
	Protocol  protocol.RouteProtocol
	AdminDist int
}

// AddRoute parses a CIDR block and injects a path into the routing table.
func (r *Router) AddRoute(cidr string, nextHop pdu.IPAddress, outInterface string, metric int, proto protocol.RouteProtocol, adminDist int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.addRouteLocked(cidr, nextHop, outInterface, metric, proto, adminDist)
}

func (r *Router) addRouteLocked(cidr string, nextHop pdu.IPAddress, outInterface string, metric int, proto protocol.RouteProtocol, adminDist int) error {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	for i, entry := range r.RoutingTable {
		if entry.Network.String() == ipNet.String() && entry.Protocol == proto {
			r.RoutingTable[i] = RouteEntry{
				Network:   ipNet,
				NextHop:   nextHop,
				Interface: outInterface,
				Metric:    metric,
				Protocol:  proto,
				AdminDist: adminDist,
			}
			return nil
		}
	}

	r.RoutingTable = append(r.RoutingTable, RouteEntry{
		Network:   ipNet,
		NextHop:   nextHop,
		Interface: outInterface,
		Metric:    metric,
		Protocol:  proto,
		AdminDist: adminDist,
	})
	return nil
}

// RemoveRoutesByProtocol removes all routes learned via a specific protocol.
func (r *Router) RemoveRoutesByProtocol(proto protocol.RouteProtocol) {
	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := make([]RouteEntry, 0, len(r.RoutingTable))
	for _, entry := range r.RoutingTable {
		if entry.Protocol != proto {
			filtered = append(filtered, entry)
		}
	}
	r.RoutingTable = filtered
}

// GetConnectedSubnets returns directly connected network CIDRs for LSA generation.
func (r *Router) GetConnectedSubnets() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subnets := make(map[string]int)
	for portID, ip := range r.Interfaces {
		mask := r.InterfaceMask[portID]
		if mask == "" {
			mask = "255.255.255.0"
		}
		subnets[ipToCIDR(ip, mask)] = 0
	}
	return subnets
}

// ResolveOspfNextHop finds the output interface and next-hop IP for an OSPF route.
func (r *Router) ResolveOspfNextHop(neighborRouterIP pdu.IPAddress, nextHopRouterID string) (string, pdu.IPAddress) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for portID, ip := range r.Interfaces {
		if string(ip) == nextHopRouterID || ip == neighborRouterIP {
			return portID, ip
		}
	}

	for portID, neighbor := range r.PortNeighbors {
		if neighbor.RouterID == nextHopRouterID {
			return portID, r.Interfaces[portID]
		}
	}

	return "", ""
}

// ResolveSubnetNextHop resolves the next-hop for a remote subnet via an OSPF transit router.
func (r *Router) ResolveSubnetNextHop(nextHopRouterID string) (pdu.IPAddress, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for portID, ip := range r.Interfaces {
		if string(ip) == nextHopRouterID {
			return ip, portID
		}
	}

	for portID, neighbor := range r.PortNeighbors {
		if string(neighbor.RemoteIP) == nextHopRouterID || neighbor.RouterID == nextHopRouterID {
			return neighbor.RemoteIP, portID
		}
	}

	return "", ""
}

// EnableOspf starts an OSPF process on this router.
func (r *Router) EnableOspf(processID int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Ospf == nil {
		routerID := r.pickRouterID()
		r.Ospf = protocol.NewOspfDaemon(routerID, processID)
	}
	r.Ospf.Enable()
}

// ConfigureOspfNetworks applies network statements and enables matching interfaces.
func (r *Router) ConfigureOspfNetworks(networks []protocol.OspfNetwork) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Ospf == nil {
		return
	}

	for _, netStmt := range networks {
		r.Ospf.AddNetwork(netStmt.CIDR, netStmt.Area)
	}

	for portID, ip := range r.Interfaces {
		mask := r.InterfaceMask[portID]
		if netStmt, matched := r.Ospf.MatchInterface(ip, mask); matched {
			r.Ospf.EnableInterface(portID, ip, mask, netStmt.Area)
		}
	}
}

// EnableRip starts a RIP process on this router.
func (r *Router) EnableRip() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Rip == nil {
		routerID := r.pickRouterID()
		r.Rip = protocol.NewRipDaemon(routerID)
	}
	r.Rip.Enable()
}

// ConfigureRipNetworks applies RIP network statements.
func (r *Router) ConfigureRipNetworks(networks []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Rip == nil {
		return
	}

	for _, cidr := range networks {
		r.Rip.AddNetwork(cidr)
	}
}

// BuildRipAdvertisement builds directly connected routes for RIP redistribution.
func (r *Router) BuildRipAdvertisement() []protocol.RipRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make([]protocol.RipRoute, 0)
	for portID, ip := range r.Interfaces {
		mask := r.InterfaceMask[portID]
		if mask == "" {
			mask = "255.255.255.0"
		}
		cidr := ipToCIDR(ip, mask)
		routes = append(routes, protocol.RipRoute{
			Network:   cidr,
			NextHop:   pdu.IPAddress("0.0.0.0"),
			Interface: portID,
			Metric:    1,
		})
	}

	if r.Rip != nil {
		for _, ripRoute := range r.Rip.GetRoutes() {
			routes = append(routes, ripRoute)
		}
	}

	return routes
}

// EnableEigrp starts an EIGRP process.
func (r *Router) EnableEigrp(asNumber int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Eigrp == nil {
		r.Eigrp = protocol.NewEigrpDaemon(r.pickRouterID(), asNumber)
	}
	r.Eigrp.Enable()
}

// ConfigureEigrpNetworks applies EIGRP network statements.
func (r *Router) ConfigureEigrpNetworks(networks []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Eigrp == nil {
		return
	}
	for _, cidr := range networks {
		r.Eigrp.AddNetwork(cidr)
	}
}

// BuildEigrpAdvertisement builds connected routes for EIGRP.
func (r *Router) BuildEigrpAdvertisement() []protocol.EigrpRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()
	routes := make([]protocol.EigrpRoute, 0)
	for portID, ip := range r.Interfaces {
		mask := r.InterfaceMask[portID]
		if mask == "" {
			mask = "255.255.255.0"
		}
		routes = append(routes, protocol.EigrpRoute{
			Network: ipToCIDR(ip, mask), Interface: portID, Metric: 100,
		})
	}
	return routes
}

// EnableBgp starts a BGP process.
func (r *Router) EnableBgp(localAS int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Bgp == nil {
		r.Bgp = protocol.NewBgpDaemon(r.pickRouterID(), localAS)
	}
	r.Bgp.Enable()
}

// AddBgpPeer adds a BGP neighbor.
func (r *Router) AddBgpPeer(peerIP pdu.IPAddress, remoteAS int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Bgp == nil {
		return
	}
	r.Bgp.AddPeer(peerIP, remoteAS)
}

// BuildBgpAdvertisement builds routes for BGP advertisement.
func (r *Router) BuildBgpAdvertisement() []protocol.BGPRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()
	routes := make([]protocol.BGPRoute, 0)
	if r.Bgp == nil {
		return routes
	}
	for portID, ip := range r.Interfaces {
		mask := r.InterfaceMask[portID]
		if mask == "" {
			mask = "255.255.255.0"
		}
		routes = append(routes, protocol.BGPRoute{
			Prefix: ipToCIDR(ip, mask), NextHop: ip, ASPath: []int{r.Bgp.LocalAS}, Origin: "IGP",
		})
	}
	return routes
}

func (r *Router) pickRouterID() pdu.IPAddress {
	var best pdu.IPAddress
	for _, ip := range r.Interfaces {
		if best == "" || string(ip) < string(best) {
			best = ip
		}
	}
	if best == "" {
		return pdu.IPAddress("1.1.1.1")
	}
	return best
}

// FormatRouteTable returns JSON-friendly routing table rows.
func (r *Router) FormatRouteTable() []RouteTableRow {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows := make([]RouteTableRow, 0, len(r.RoutingTable))
	for _, entry := range r.RoutingTable {
		nextHop := string(entry.NextHop)
		if entry.Protocol == protocol.RouteConnected {
			nextHop = "directly connected"
		}
		rows = append(rows, RouteTableRow{
			Protocol:  string(entry.Protocol),
			Network:   entry.Network.String(),
			Metric:    entry.Metric,
			NextHop:   nextHop,
			Interface: entry.Interface,
		})
	}
	return rows
}

// MatchRoute evaluates the routing table using longest prefix matching.
func (r *Router) MatchRoute(destIP pdu.IPAddress) (*RouteEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targetIP := net.ParseIP(string(destIP))
	var bestMatch *RouteEntry
	bestMaskLen := -1
	bestAdminDist := 999999

	for _, entry := range r.RoutingTable {
		if entry.Network.Contains(targetIP) {
			ones, _ := entry.Network.Mask.Size()
			if ones > bestMaskLen || (ones == bestMaskLen && entry.AdminDist < bestAdminDist) {
				bestMaskLen = ones
				bestAdminDist = entry.AdminDist
				tempEntry := entry
				bestMatch = &tempEntry
			}
		}
	}
	return bestMatch, bestMatch != nil
}

// Snapshot exports router configuration for persistence.
func (r *Router) Snapshot() RouterSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snap := RouterSnapshot{ID: r.ID}
	for portID, ip := range r.Interfaces {
		snap.Interfaces = append(snap.Interfaces, IfaceSnapshot{
			PortID: portID,
			IP:     string(ip),
			Mask:   r.InterfaceMask[portID],
			MAC:    string(r.InterfaceMAC[portID]),
		})
	}
	for _, entry := range r.RoutingTable {
		if entry.Protocol == protocol.RouteStatic {
			snap.StaticRoutes = append(snap.StaticRoutes, StaticRouteSnap{
				CIDR:      entry.Network.String(),
				NextHop:   string(entry.NextHop),
				Interface: entry.Interface,
				Metric:    entry.Metric,
			})
		}
	}
	if r.Ospf != nil && r.Ospf.Enabled {
		snap.OspfEnabled = true
		snap.OspfProcessID = r.Ospf.ProcessID
		for _, n := range r.Ospf.Networks {
			snap.OspfNetworks = append(snap.OspfNetworks, OspfNetworkSnap{CIDR: n.CIDR, Area: n.Area})
		}
	}
	if r.Rip != nil && r.Rip.Enabled {
		snap.RipEnabled = true
		snap.RipNetworks = append(snap.RipNetworks, r.Rip.Networks...)
	}
	if r.Eigrp != nil && r.Eigrp.Enabled {
		snap.EigrpEnabled = true
		snap.EigrpAS = r.Eigrp.ASNumber
		snap.EigrpNetworks = append(snap.EigrpNetworks, r.Eigrp.Networks...)
	}
	if r.Bgp != nil && r.Bgp.Enabled {
		snap.BgpEnabled = true
		snap.BgpLocalAS = r.Bgp.LocalAS
	}
	return snap
}

// RestoreRouter creates a router from a snapshot.
func RestoreRouter(snap RouterSnapshot) *Router {
	r := NewRouter(snap.ID)
	for _, iface := range snap.Interfaces {
		r.AddInterface(iface.PortID, pdu.IPAddress(iface.IP), iface.Mask, pdu.MACAddress(iface.MAC))
	}
	for _, sr := range snap.StaticRoutes {
		_ = r.AddRoute(sr.CIDR, pdu.IPAddress(sr.NextHop), sr.Interface, sr.Metric, protocol.RouteStatic, protocol.AdminDistStatic)
	}
	if snap.OspfEnabled {
		r.EnableOspf(snap.OspfProcessID)
		networks := make([]protocol.OspfNetwork, 0, len(snap.OspfNetworks))
		for _, n := range snap.OspfNetworks {
			networks = append(networks, protocol.OspfNetwork{CIDR: n.CIDR, Area: n.Area})
		}
		r.ConfigureOspfNetworks(networks)
	}
	if snap.RipEnabled {
		r.EnableRip()
		r.ConfigureRipNetworks(snap.RipNetworks)
	}
	if snap.EigrpEnabled {
		r.EnableEigrp(snap.EigrpAS)
		r.ConfigureEigrpNetworks(snap.EigrpNetworks)
	}
	if snap.BgpEnabled {
		r.EnableBgp(snap.BgpLocalAS)
	}
	return r
}

func ipToCIDR(ip pdu.IPAddress, mask string) string {
	ipAddr := net.ParseIP(string(ip))
	maskAddr := net.IPMask(net.ParseIP(mask).To4())
	if ipAddr == nil || maskAddr == nil {
		return string(ip) + "/24"
	}
	return (&net.IPNet{IP: ipAddr.Mask(maskAddr), Mask: maskAddr}).String()
}
