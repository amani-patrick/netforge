package network

import (
	"fmt"

	"netforge/engine/pdu"
)

// HSRPState is the redundancy role.
type HSRPState string

const (
	HSRPActive  HSRPState = "Active"
	HSRPStandby HSRPState = "Standby"
	HSRPListen  HSRPState = "Listen"
)

// HSRPGroup is a Hot Standby Router Protocol group on an interface.
type HSRPGroup struct {
	GroupID   int
	Interface string
	VirtualIP pdu.IPAddress
	Priority  int
	Preempt   bool
	State     HSRPState
	HelloTime int
}

func hsrpKey(portID string, groupID int) string {
	return fmt.Sprintf("%s:%d", portID, groupID)
}

// ConfigureHSRP sets HSRP parameters on a router interface.
func (r *Router) ConfigureHSRP(portID string, groupID int, virtualIP pdu.IPAddress, priority int, preempt bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.HSRPGroups == nil {
		r.HSRPGroups = make(map[string]*HSRPGroup)
	}
	r.HSRPGroups[hsrpKey(portID, groupID)] = &HSRPGroup{
		GroupID: groupID, Interface: portID, VirtualIP: virtualIP,
		Priority: priority, Preempt: preempt, State: HSRPListen, HelloTime: 3,
	}
}

// OwnsHSRPVirtualIP returns true if this router is active for the virtual IP.
func (r *Router) OwnsHSRPVirtualIP(ip pdu.IPAddress) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, g := range r.HSRPGroups {
		if g.VirtualIP == ip && g.State == HSRPActive {
			return true
		}
	}
	return false
}

// SetHostname sets the IOS hostname.
func (r *Router) SetHostname(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Hostname = name
}

// GetHostname returns router hostname or ID.
func (r *Router) GetHostname() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Hostname != "" {
		return r.Hostname
	}
	return r.ID
}

// RunHSRPElection elects active/standby routers per shared virtual IP on a LAN.
func (m *Manager) RunHSRPElection() {
	m.mu.RLock()
	routers := make([]*Router, 0, len(m.Routers))
	for _, r := range m.Routers {
		routers = append(routers, r)
	}
	m.mu.RUnlock()

	type candidate struct {
		routerID string
		group    *HSRPGroup
		priority int
	}
	byVIP := make(map[string][]candidate)

	for _, router := range routers {
		router.mu.Lock()
		for _, g := range router.HSRPGroups {
			byVIP[string(g.VirtualIP)] = append(byVIP[string(g.VirtualIP)], candidate{router.ID, g, g.Priority})
		}
		router.mu.Unlock()
	}

	for vip, candidates := range byVIP {
		if len(candidates) == 0 {
			continue
		}
		best := candidates[0]
		for _, c := range candidates[1:] {
			if c.priority > best.priority {
				best = c
			}
		}
		for _, c := range candidates {
			if c.routerID == best.routerID && c.group.GroupID == best.group.GroupID {
				c.group.State = HSRPActive
			} else if c.group.State != HSRPActive {
				c.group.State = HSRPStandby
			}
		}
		m.LogEvent(EventProtocol, best.routerID, best.group.Interface,
			"HSRP elected Active for "+vip, nil)
	}
}

// HSRPStatusRow is JSON output for show standby.
type HSRPStatusRow struct {
	Interface string `json:"interface"`
	Group     int    `json:"group"`
	State     string `json:"state"`
	VirtualIP string `json:"virtual_ip"`
	Priority  int    `json:"priority"`
	ActiveIP  string `json:"active_ip"`
}

// FormatHSRPStatus returns HSRP table for a router.
func (r *Router) FormatHSRPStatus() []HSRPStatusRow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rows := make([]HSRPStatusRow, 0, len(r.HSRPGroups))
	for _, g := range r.HSRPGroups {
		activeIP := ""
		if g.State == HSRPActive {
			activeIP = string(r.Interfaces[g.Interface])
		}
		rows = append(rows, HSRPStatusRow{
			Interface: g.Interface, Group: g.GroupID, State: string(g.State),
			VirtualIP: string(g.VirtualIP), Priority: g.Priority, ActiveIP: activeIP,
		})
	}
	return rows
}

// AddDHCPExcludedRange excludes an IP range from DHCP pools on a router.
func (r *Router) AddDHCPExcludedRange(start, end string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.DHCPExcluded == nil {
		r.DHCPExcluded = make([]string, 0)
	}
	r.DHCPExcluded = append(r.DHCPExcluded, start+" "+end)
	for _, pool := range r.DHCPPools {
		pool.Excluded = append(pool.Excluded, start+" "+end)
	}
}
