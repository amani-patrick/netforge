package network

import (
	"fmt"
	"sync"
	"time"

	"netforge/engine/pdu"
)

// QoSMatchType classifies traffic in a class-map.
type QoSMatchType string

const (
	MatchDSCP        QoSMatchType = "dscp"
	MatchProtocol    QoSMatchType = "protocol"
	MatchAccessGroup QoSMatchType = "access-group"
)

// QoSClassMap matches traffic for policy actions.
type QoSClassMap struct {
	Name      string
	MatchType QoSMatchType
	MatchVal  string
}

// QoSPolicyAction defines shaping/priority behavior.
type QoSPolicyAction struct {
	Type  string // priority, bandwidth, police, set-dscp
	Value int
	Unit  string
}

// QoSPolicyClass binds a class-map to actions inside a policy-map.
type QoSPolicyClass struct {
	ClassMap string
	Actions  []QoSPolicyAction
}

// QoSPolicyMap is a hierarchical QoS policy.
type QoSPolicyMap struct {
	Name    string
	Classes []QoSPolicyClass
	Default []QoSPolicyAction
}

// QoSServicePolicy binds a policy-map to an interface direction.
type QoSServicePolicy struct {
	PolicyName string
	Direction  string
}

// AddClassMap creates a class-map on a router.
func (r *Router) AddClassMap(cm QoSClassMap) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ClassMaps == nil {
		r.ClassMaps = make(map[string]*QoSClassMap)
	}
	c := cm
	r.ClassMaps[cm.Name] = &c
}

// AddPolicyMap creates a policy-map on a router.
func (r *Router) AddPolicyMap(pm QoSPolicyMap) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.PolicyMaps == nil {
		r.PolicyMaps = make(map[string]*QoSPolicyMap)
	}
	p := pm
	r.PolicyMaps[pm.Name] = &p
}

// ApplyServicePolicy attaches a policy-map to an interface.
func (r *Router) ApplyServicePolicy(portID, policyName, direction string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.IfaceQoS == nil {
		r.IfaceQoS = make(map[string]*QoSServicePolicy)
	}
	r.IfaceQoS[portID] = &QoSServicePolicy{PolicyName: policyName, Direction: direction}
}

// QoSPoliceBucket tracks token-bucket policing per interface/class.
type QoSPoliceBucket struct {
	Tokens     float64
	LastUpdate time.Duration
	RateKbps   int
}

// ApplyQoSToPacket classifies and marks an IP packet per interface policy.
// Returns DSCP marking and whether the packet should be dropped by police.
func (r *Router) ApplyQoSToPacket(portID string, ip *pdu.IPv4Packet, simTime time.Duration) (dscp int, drop bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	sp := r.IfaceQoS[portID]
	if sp == nil {
		return pdu.DSCPDefault, false
	}
	pm := r.PolicyMaps[sp.PolicyName]
	if pm == nil {
		return pdu.DSCPDefault, false
	}
	for _, pc := range pm.Classes {
		cm := r.ClassMaps[pc.ClassMap]
		if cm == nil {
			continue
		}
		if !matchesClassMap(cm, ip) {
			continue
		}
		for _, act := range pc.Actions {
			switch act.Type {
			case "set-dscp":
				dscp = act.Value
			case "priority":
				dscp = pdu.DSCPEF
			case "police":
				if r.shouldPoliceDrop(portID, pc.ClassMap, act.Value, simTime) {
					return dscp, true
				}
			}
		}
		if dscp == 0 {
			dscp = pdu.DSCPDefault
		}
		return dscp, false
	}
	return pdu.DSCPDefault, false
}

func (r *Router) shouldPoliceDrop(portID, className string, rateKbps int, simTime time.Duration) bool {
	if r.qosPolice == nil {
		r.qosPolice = make(map[string]map[string]*QoSPoliceBucket)
	}
	if r.qosPolice[portID] == nil {
		r.qosPolice[portID] = make(map[string]*QoSPoliceBucket)
	}
	bucket := r.qosPolice[portID][className]
	if bucket == nil {
		bucket = &QoSPoliceBucket{Tokens: float64(rateKbps), RateKbps: rateKbps, LastUpdate: simTime}
		r.qosPolice[portID][className] = bucket
	}
	elapsed := simTime - bucket.LastUpdate
	if elapsed > 0 {
		bucket.Tokens += float64(rateKbps) * elapsed.Seconds() / 8.0
		if bucket.Tokens > float64(rateKbps) {
			bucket.Tokens = float64(rateKbps)
		}
		bucket.LastUpdate = simTime
	}
	if bucket.Tokens < 1 {
		return true
	}
	bucket.Tokens--
	return false
}

// QoSBandwidthShare returns delay multiplier for non-priority traffic (1.0 = normal).
func (r *Router) QoSBandwidthShare(portID string, ip *pdu.IPv4Packet) float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sp := r.IfaceQoS[portID]
	if sp == nil {
		return 1.0
	}
	pm := r.PolicyMaps[sp.PolicyName]
	if pm == nil {
		return 1.0
	}
	for _, pc := range pm.Classes {
		cm := r.ClassMaps[pc.ClassMap]
		if cm == nil || !matchesClassMap(cm, ip) {
			continue
		}
		for _, act := range pc.Actions {
			if act.Type == "bandwidth" && act.Value > 0 && act.Value < 100 {
				return 1.0 + float64(100-act.Value)/25.0
			}
		}
	}
	return 1.0
}

// AppendPolicyClass adds or updates a class inside a policy-map (config mode).
func (r *Router) AppendPolicyClass(policyName string, pc QoSPolicyClass) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.PolicyMaps == nil {
		r.PolicyMaps = make(map[string]*QoSPolicyMap)
	}
	pm, ok := r.PolicyMaps[policyName]
	if !ok {
		pm = &QoSPolicyMap{Name: policyName}
		r.PolicyMaps[policyName] = pm
	}
	for i, existing := range pm.Classes {
		if existing.ClassMap == pc.ClassMap {
			pm.Classes[i].Actions = append(pm.Classes[i].Actions, pc.Actions...)
			return
		}
	}
	pm.Classes = append(pm.Classes, pc)
}

// AppendClassMapMatch updates class-map match criteria.
func (r *Router) AppendClassMapMatch(name string, matchType QoSMatchType, matchVal string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ClassMaps == nil {
		r.ClassMaps = make(map[string]*QoSClassMap)
	}
	cm, ok := r.ClassMaps[name]
	if !ok {
		cm = &QoSClassMap{Name: name}
		r.ClassMaps[name] = cm
	}
	cm.MatchType = matchType
	cm.MatchVal = matchVal
}

func matchesClassMap(cm *QoSClassMap, ip *pdu.IPv4Packet) bool {
	switch cm.MatchType {
	case MatchProtocol:
		if cm.MatchVal == "voice" || cm.MatchVal == "sip" {
			if ip.Protocol != pdu.ProtoUDP {
				return false
			}
			return ip.SIP != nil || ip.DstPort == pdu.PortSIP || ip.SrcPort == pdu.PortSIP ||
				(ip.DstPort == 0 && ip.SrcPort == 0)
		}
		if cm.MatchVal == "icmp" {
			return ip.Protocol == pdu.ProtoICMP
		}
	case MatchDSCP:
		return true
	}
	return false
}

// IsHighPriorityPacket returns true if packet should use priority queue.
func (r *Router) IsHighPriorityPacket(portID string, ip *pdu.IPv4Packet, dscp int) bool {
	if dscp >= pdu.DSCPEF {
		return true
	}
	return ip.Protocol == pdu.ProtoUDP && dscp >= pdu.DSCPAF41
}

// SwitchQoSPolicy is MLS QoS settings per port.
type SwitchQoSPolicy struct {
	TrustDSCP   bool
	TrustCos    bool
	QueuePolicy string
}

// ApplySwitchQoSPolicy sets MLS QoS on a switch port.
func (sw *Switch) ApplySwitchQoSPolicy(portID string, policy SwitchQoSPolicy) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.ensurePort(portID)
	if sw.PortQoS == nil {
		sw.PortQoS = make(map[string]*SwitchQoSPolicy)
	}
	p := policy
	sw.PortQoS[portID] = &p
	if policy.TrustDSCP {
		sw.Ports[portID].QoSPriority = 5
	}
}

// ClassifySwitchFrame returns queue priority for a frame on a port.
func (sw *Switch) ClassifySwitchFrame(portID string, dscp int) int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	if sw.PortQoS != nil {
		if pol, ok := sw.PortQoS[portID]; ok && pol.TrustDSCP && dscp >= pdu.DSCPEF {
			return 1
		}
	}
	if cfg := sw.Ports[portID]; cfg != nil && cfg.VoiceEnabled {
		return 1
	}
	return 0
}

// FormatQoSPolicy returns policy-map summary for show commands.
func (r *Router) FormatQoSPolicy() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	lines := make([]string, 0)
	for name, pm := range r.PolicyMaps {
		lines = append(lines, "Policy Map "+name)
		for _, c := range pm.Classes {
			lines = append(lines, "  Class "+c.ClassMap)
			for _, a := range c.Actions {
				lines = append(lines, fmt.Sprintf("    %s %d %s", a.Type, a.Value, a.Unit))
			}
		}
	}
	return lines
}

var _ = sync.Mutex{}
