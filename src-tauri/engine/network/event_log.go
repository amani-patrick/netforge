package network

import (
	"sync"
	"time"
)

// EventCategory classifies simulation trace entries.
type EventCategory string

const (
	EventPacketTX    EventCategory = "PACKET_TX"
	EventPacketRX    EventCategory = "PACKET_RX"
	EventPacketDrop  EventCategory = "PACKET_DROP"
	EventRouteChange EventCategory = "ROUTE_CHANGE"
	EventOSPF        EventCategory = "OSPF"
	EventRIP         EventCategory = "RIP"
	EventACLDeny     EventCategory = "ACL_DENY"
	EventLink        EventCategory = "LINK"
	EventProtocol    EventCategory = "PROTOCOL"
	EventAssessment  EventCategory = "ASSESSMENT"
)

// EventLogEntry is one trace line in the simulation event log.
type EventLogEntry struct {
	SimTime  int64                  `json:"sim_time_ms"`
	Category EventCategory          `json:"category"`
	NodeID   string                 `json:"node_id,omitempty"`
	PortID   string                 `json:"port_id,omitempty"`
	Message  string                 `json:"message"`
	Details  map[string]interface{} `json:"details,omitempty"`
}

// EventLog is a ring buffer of simulation events for debugging and UI.
type EventLog struct {
	entries []EventLogEntry
	maxSize int
	mu      sync.RWMutex
}

// NewEventLog creates an event log with a capacity limit.
func NewEventLog(maxSize int) *EventLog {
	if maxSize <= 0 {
		maxSize = 10000
	}
	return &EventLog{maxSize: maxSize}
}

// Record appends an event at the current simulation time.
func (l *EventLog) Record(simTime time.Duration, category EventCategory, nodeID, portID, message string, details map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := EventLogEntry{
		SimTime:  simTime.Milliseconds(),
		Category: category,
		NodeID:   nodeID,
		PortID:   portID,
		Message:  message,
		Details:  details,
	}
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxSize {
		l.entries = l.entries[len(l.entries)-l.maxSize:]
	}
}

// Get returns the most recent entries, optionally filtered by category.
func (l *EventLog) Get(limit int, category EventCategory) []EventLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if limit <= 0 || limit > len(l.entries) {
		limit = len(l.entries)
	}
	start := len(l.entries) - limit
	if start < 0 {
		start = 0
	}
	slice := l.entries[start:]
	if category == "" {
		out := make([]EventLogEntry, len(slice))
		copy(out, slice)
		return out
	}
	out := make([]EventLogEntry, 0)
	for _, e := range slice {
		if e.Category == category {
			out = append(out, e)
		}
	}
	return out
}

// Clear removes all entries.
func (l *EventLog) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = nil
}

// LogEvent records a simulation event on the manager.
func (m *Manager) LogEvent(category EventCategory, nodeID, portID, message string, details map[string]interface{}) {
	if m.eventLog == nil {
		return
	}
	m.eventLog.Record(m.SimNow(), category, nodeID, portID, message, details)
}

// GetEventLog returns recent trace entries.
func (m *Manager) GetEventLog(limit int, category EventCategory) []EventLogEntry {
	if m.eventLog == nil {
		return nil
	}
	return m.eventLog.Get(limit, category)
}

// ClearEventLog wipes the trace buffer.
func (m *Manager) ClearEventLog() {
	if m.eventLog != nil {
		m.eventLog.Clear()
	}
}
