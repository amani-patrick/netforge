package protocol

import (
	"sync"

	"netforge/engine/pdu"
)

const (
	RouteBGP   RouteProtocol = "B"
	AdminDistBGP = 20
)

// BGPPeer is a BGP neighbor session.
type BGPPeer struct {
	PeerIP   pdu.IPAddress
	RemoteAS int
	LocalAS  int
	State    string
}

// BGPRoute is a BGP path advertisement.
type BGPRoute struct {
	Prefix  string
	NextHop pdu.IPAddress
	ASPath  []int
	Origin  string
}

// BgpDaemon runs simplified eBGP.
type BgpDaemon struct {
	LocalAS  int
	RouterID pdu.IPAddress
	Enabled  bool
	Peers    map[pdu.IPAddress]*BGPPeer
	Routes   map[string]*BGPRoute
	mu       sync.RWMutex
}

// NewBgpDaemon creates a BGP process.
func NewBgpDaemon(routerID pdu.IPAddress, localAS int) *BgpDaemon {
	return &BgpDaemon{
		LocalAS: localAS, RouterID: routerID,
		Peers: make(map[pdu.IPAddress]*BGPPeer),
		Routes: make(map[string]*BGPRoute),
	}
}

func (b *BgpDaemon) Enable() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Enabled = true
}

func (b *BgpDaemon) AddPeer(peerIP pdu.IPAddress, remoteAS int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Peers[peerIP] = &BGPPeer{
		PeerIP: peerIP, RemoteAS: remoteAS, LocalAS: b.LocalAS, State: "Established",
	}
}

func (b *BgpDaemon) ProcessUpdate(routes []BGPRoute, peer pdu.IPAddress) []BGPRoute {
	b.mu.Lock()
	defer b.mu.Unlock()

	updated := make([]BGPRoute, 0)
	peerCfg, ok := b.Peers[peer]
	if !ok {
		return updated
	}

	for _, route := range routes {
		if len(route.ASPath) > 0 && route.ASPath[len(route.ASPath)-1] == peerCfg.RemoteAS {
			continue
		}
		newPath := append(route.ASPath, peerCfg.RemoteAS)
		existing, found := b.Routes[route.Prefix]
		if !found || len(newPath) < len(existing.ASPath) {
			r := BGPRoute{Prefix: route.Prefix, NextHop: peer, ASPath: newPath, Origin: "IGP"}
			b.Routes[route.Prefix] = &r
			updated = append(updated, r)
		}
	}
	return updated
}

func (b *BgpDaemon) GetAdvertisement(local []BGPRoute) []BGPRoute {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := append([]BGPRoute{}, local...)
	for _, r := range b.Routes {
		out = append(out, *r)
	}
	return out
}
