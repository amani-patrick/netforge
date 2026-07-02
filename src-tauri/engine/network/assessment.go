package network

import "netforge/engine/pdu"

import (
	"fmt"
)

// GoalType identifies an activity check.
type GoalType string

const (
	GoalPing          GoalType = "ping"
	GoalRouteExists   GoalType = "route_exists"
	GoalOspfNeighbor  GoalType = "ospf_neighbor"
	GoalACLConfigured GoalType = "acl_configured"
	GoalDeviceExists  GoalType = "device_exists"
	GoalVLANConfigured GoalType = "vlan_configured"
	GoalHSRPActive    GoalType = "hsrp_active"
	GoalNATConfigured GoalType = "nat_configured"
	GoalDHCPAssigned  GoalType = "dhcp_assigned"
	GoalTrunkConfigured GoalType = "trunk_configured"
	GoalVPNActive       GoalType = "vpn_active"
)

// ActivityGoal is a gradable lab objective.
type ActivityGoal struct {
	ID          string            `json:"id"`
	Type        GoalType          `json:"type"`
	Description string            `json:"description"`
	Params      map[string]string `json:"params"`
}

// ActivityResult is the outcome of evaluating one goal.
type ActivityResult struct {
	GoalID      string `json:"goal_id"`
	Passed      bool   `json:"passed"`
	Message     string `json:"message"`
	Description string `json:"description,omitempty"`
}

// ActivityEngine grades topology against defined goals.
type ActivityEngine struct {
	Goals []ActivityGoal
}

// NewActivityEngine creates an empty assessment engine.
func NewActivityEngine() *ActivityEngine {
	return &ActivityEngine{Goals: make([]ActivityGoal, 0)}
}

// AddGoal registers a lab objective.
func (a *ActivityEngine) AddGoal(goal ActivityGoal) {
	a.Goals = append(a.Goals, goal)
}

// ClearGoals removes all objectives.
func (a *ActivityEngine) ClearGoals() {
	a.Goals = nil
}

// Evaluate runs all goals against current simulation state.
func (m *Manager) EvaluateActivity() []ActivityResult {
	if m.activity == nil {
		return nil
	}
	results := make([]ActivityResult, 0, len(m.activity.Goals))
	for _, goal := range m.activity.Goals {
		results = append(results, m.evaluateGoal(goal))
	}
	return results
}

func (m *Manager) evaluateGoal(goal ActivityGoal) ActivityResult {
	result := ActivityResult{GoalID: goal.ID, Description: goal.Description}
	switch goal.Type {
	case GoalPing:
		result.Passed, result.Message = m.checkPingGoal(goal.Params)
	case GoalRouteExists:
		result.Passed, result.Message = m.checkRouteGoal(goal.Params)
	case GoalOspfNeighbor:
		result.Passed, result.Message = m.checkOspfNeighborGoal(goal.Params)
	case GoalACLConfigured:
		result.Passed, result.Message = m.checkACLGoal(goal.Params)
	case GoalDeviceExists:
		result.Passed, result.Message = m.checkDeviceGoal(goal.Params)
	case GoalVLANConfigured:
		result.Passed, result.Message = m.checkVLANGoal(goal.Params)
	case GoalHSRPActive:
		result.Passed, result.Message = m.checkHSRPGoal(goal.Params)
	case GoalNATConfigured:
		result.Passed, result.Message = m.checkNATGoal(goal.Params)
	case GoalDHCPAssigned:
		result.Passed, result.Message = m.checkDHCPGoal(goal.Params)
	case GoalTrunkConfigured:
		result.Passed, result.Message = m.checkTrunkGoal(goal.Params)
	case GoalVPNActive:
		result.Passed, result.Message = m.checkVPNGoal(goal.Params)
	default:
		result.Message = fmt.Sprintf("unknown goal type: %s", goal.Type)
	}
	if result.Passed {
		m.LogEvent(EventAssessment, "", "", fmt.Sprintf("goal %s passed", goal.ID), nil)
	}
	return result
}

func (m *Manager) checkPingGoal(p map[string]string) (bool, string) {
	sourceID := p["source_id"]
	destIP := p["dest_ip"]
	host, ok := m.GetHost(sourceID)
	if !ok {
		return false, fmt.Sprintf("host %s not found", sourceID)
	}
	if host.IsLocalAddress(pduIPAddress(destIP)) {
		return true, fmt.Sprintf("%s can reach local %s", sourceID, destIP)
	}
	_, found := m.findRouteToHost(host, destIP)
	if !found {
		return false, fmt.Sprintf("no route from %s to %s", sourceID, destIP)
	}
	return true, fmt.Sprintf("route exists from %s to %s", sourceID, destIP)
}

func (m *Manager) checkVLANGoal(p map[string]string) (bool, string) {
	switchID := p["switch_id"]
	vlanID := p["vlan_id"]
	sw, ok := m.GetSwitch(switchID)
	if !ok {
		return false, fmt.Sprintf("switch %s not found", switchID)
	}
	vlan, err := parseVLANID(vlanID)
	if err != nil {
		return false, err.Error()
	}
	if sw.HasVLAN(vlan) {
		return true, fmt.Sprintf("VLAN %d exists on %s", vlan, switchID)
	}
	return false, fmt.Sprintf("VLAN %d not configured on %s", vlan, switchID)
}

func (m *Manager) checkHSRPGoal(p map[string]string) (bool, string) {
	routerID := p["router_id"]
	groupID := p["group_id"]
	r, ok := m.GetRouter(routerID)
	if !ok {
		return false, fmt.Sprintf("router %s not found", routerID)
	}
	for _, row := range r.FormatHSRPStatus() {
		if groupID != "" && fmt.Sprintf("%d", row.Group) != groupID {
			continue
		}
		if row.State == "Active" {
			return true, fmt.Sprintf("HSRP group %d active on %s", row.Group, routerID)
		}
	}
	return false, "no active HSRP group"
}

func (m *Manager) checkNATGoal(p map[string]string) (bool, string) {
	routerID := p["router_id"]
	r, ok := m.GetRouter(routerID)
	if !ok {
		return false, fmt.Sprintf("router %s not found", routerID)
	}
	if r.NAT != nil && (r.NAT.Overload || len(r.NAT.Static) > 0) {
		return true, fmt.Sprintf("NAT configured on %s", routerID)
	}
	return false, fmt.Sprintf("NAT not configured on %s", routerID)
}

func (m *Manager) checkDHCPGoal(p map[string]string) (bool, string) {
	hostID := p["host_id"]
	host, ok := m.GetHost(hostID)
	if !ok {
		return false, fmt.Sprintf("host %s not found", hostID)
	}
	if host.IP != "" {
		return true, fmt.Sprintf("host %s has IP %s", hostID, host.IP)
	}
	return false, fmt.Sprintf("host %s has no IP", hostID)
}

func (m *Manager) checkTrunkGoal(p map[string]string) (bool, string) {
	switchID := p["switch_id"]
	portID := p["port_id"]
	sw, ok := m.GetSwitch(switchID)
	if !ok {
		return false, fmt.Sprintf("switch %s not found", switchID)
	}
	cfg := sw.GetPortConfig(portID)
	if cfg.Mode == PortModeTrunk {
		return true, fmt.Sprintf("port %s is trunk on %s", portID, switchID)
	}
	return false, fmt.Sprintf("port %s is not trunk", portID)
}

func (m *Manager) checkVPNGoal(p map[string]string) (bool, string) {
	routerID := p["router_id"]
	r, ok := m.GetRouter(routerID)
	if !ok {
		return false, fmt.Sprintf("router %s not found", routerID)
	}
	if r.HasActiveVPN() {
		return true, fmt.Sprintf("IPsec VPN active on %s", routerID)
	}
	return false, fmt.Sprintf("no active VPN on %s", routerID)
}

func parseVLANID(s string) (pdu.VLANID, error) {
	var v int
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return 0, fmt.Errorf("invalid vlan id")
	}
	return pdu.VLANID(v), nil
}

func (m *Manager) findRouteToHost(host *Host, destIP string) (bool, bool) {
	if host.IsLocalAddress(pduIPAddress(destIP)) {
		return true, true
	}
	uplink := host.UplinkNode
	router, ok := m.GetRouter(uplink)
	if !ok {
		m.mu.RLock()
		for _, r := range m.Routers {
			router = r
			ok = true
			break
		}
		m.mu.RUnlock()
	}
	if !ok {
		return false, false
	}
	_, found := router.MatchRoute(pduIPAddress(destIP))
	return found, found
}

func pduIPAddress(s string) pdu.IPAddress { return pdu.IPAddress(s) }

func (m *Manager) checkRouteGoal(p map[string]string) (bool, string) {
	routerID := p["router_id"]
	network := p["network"]
	router, ok := m.GetRouter(routerID)
	if !ok {
		return false, fmt.Sprintf("router %s not found", routerID)
	}
	_, found := router.MatchRoute(pduIPAddress(network))
	if !found {
		return false, fmt.Sprintf("no route to %s on %s", network, routerID)
	}
	return true, fmt.Sprintf("route to %s exists on %s", network, routerID)
}

func (m *Manager) checkOspfNeighborGoal(p map[string]string) (bool, string) {
	routerID := p["router_id"]
	router, ok := m.GetRouter(routerID)
	if !ok {
		return false, fmt.Sprintf("router %s not found", routerID)
	}
	if router.Ospf == nil {
		return false, "OSPF not enabled"
	}
	neighbors := router.Ospf.GetNeighbors()
	if len(neighbors) == 0 {
		return false, "no OSPF neighbors"
	}
	if want := p["neighbor_id"]; want != "" {
		for _, n := range neighbors {
			if string(n.RouterID) == want {
				return true, fmt.Sprintf("OSPF neighbor %s is up", want)
			}
		}
		return false, fmt.Sprintf("neighbor %s not found", want)
	}
	return true, fmt.Sprintf("%d OSPF neighbor(s) present", len(neighbors))
}

func (m *Manager) checkACLGoal(p map[string]string) (bool, string) {
	routerID := p["router_id"]
	aclName := p["acl_name"]
	router, ok := m.GetRouter(routerID)
	if !ok {
		return false, fmt.Sprintf("router %s not found", routerID)
	}
	if router.ACLs == nil {
		return false, fmt.Sprintf("ACL %s not configured", aclName)
	}
	if _, ok := router.ACLs[aclName]; !ok {
		return false, fmt.Sprintf("ACL %s not configured", aclName)
	}
	return true, fmt.Sprintf("ACL %s exists on %s", aclName, routerID)
}

func (m *Manager) checkDeviceGoal(p map[string]string) (bool, string) {
	deviceID := p["device_id"]
	if _, ok := m.GetDevice(deviceID); ok {
		return true, fmt.Sprintf("device %s exists", deviceID)
	}
	return false, fmt.Sprintf("device %s not found", deviceID)
}

// AddActivityGoal registers a gradable objective.
func (m *Manager) AddActivityGoal(goal ActivityGoal) {
	if m.activity == nil {
		m.activity = NewActivityEngine()
	}
	m.activity.AddGoal(goal)
}

// ClearActivityGoals removes all objectives.
func (m *Manager) ClearActivityGoals() {
	if m.activity != nil {
		m.activity.ClearGoals()
	}
}
