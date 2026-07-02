package engine

import (
	"container/heap"
	"sync"
	"time"
)

// EventType defines what kind of action the simulation needs to execute.
type EventType string

const (
	EventPacketTx    EventType = "PACKET_TX"
	EventPacketRx    EventType = "PACKET_RX"
	EventTimerOSPF   EventType = "TIMER_OSPF"
	EventTimerRIP    EventType = "TIMER_RIP"
	EventTimerICMP   EventType = "TIMER_ICMP"
	EventTimerEIGRP  EventType = "TIMER_EIGRP"
	EventTimerBGP    EventType = "TIMER_BGP"
	EventTimerCDP    EventType = "TIMER_CDP"
	EventTimerSTP    EventType = "TIMER_STP"
	EventTimerHSRP   EventType = "TIMER_HSRP"
	EventTimerVTP    EventType = "TIMER_VTP"
	EventTimerVoIP   EventType = "TIMER_VOIP"
	EventLinkToggle  EventType = "LINK_TOGGLE"
)

// Protocol timer intervals used by the discrete-event scheduler.
const (
	OspfHelloInterval  = 10 * time.Second
	RipUpdateInterval  = 30 * time.Second
	IcmpPingTimeout    = 2 * time.Second
	EigrpUpdateInterval = 30 * time.Second
	BgpUpdateInterval  = 60 * time.Second
	CdpInterval        = 60 * time.Second
	StpInterval        = 30 * time.Second
	HsrpHelloInterval  = 3 * time.Second
	VtpAdvertInterval  = 30 * time.Second
)

// Default link physical parameters.
const (
	DefaultBandwidth   int64   = 100_000_000 // 100 Mbps
	DefaultCableLength float64 = 10.0          // meters
)

// Event represents a single point-in-time occurrence in our simulated network.
type Event struct {
	ID        uint64      // Unique event identifier
	Timestamp time.Duration // Simulation time when this event MUST execute
	Type      EventType   // What type of event this is
	Data      interface{} // Flexible payload (e.g., our PDU/Packet struct or Interface ID)
	Index     int         // Internal tracker required by container/heap
}

// PriorityQueue implements heap.Interface and holds Events.
// It ensures that the event with the earliest Timestamp is always at the top.
type PriorityQueue []*Event

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].Timestamp < pq[j].Timestamp }
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Event)
	item.Index = n
	*pq = append(*pq, item)
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // Avoid memory leak
	item.Index = -1 // For safety
	*pq = old[0 : n-1]
	return item
}

// Scheduler manages the absolute simulation time and orchestrates event processing.
type Scheduler struct {
	mu         sync.Mutex
	pq         PriorityQueue
	simClock   time.Duration // The virtual timeline clock
	eventCount uint64
	isRunning  bool
}

// NewScheduler creates a clean, ready-to-use discrete event scheduler.
func NewScheduler() *Scheduler {
	s := &Scheduler{
		pq:       make(PriorityQueue, 0),
		simClock: 0,
	}
	heap.Init(&s.pq)
	return s
}

// Schedule inserts a new network event into the timeline.
// offset: How far into the virtual future this event happens (e.g., +5ms for cable propagation).
func (s *Scheduler) Schedule(eventType EventType, offset time.Duration, data interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.eventCount++
	event := &Event{
		ID:        s.eventCount,
		Timestamp: s.simClock + offset,
		Type:      eventType,
		Data:      data,
	}
	heap.Push(&s.pq, event)
}

// Step processes the single next chronological event in the queue.
// Returns the event processed, and a boolean indicating if an event was actually handled.
func (s *Scheduler) Step() (*Event, bool) {
	s.mu.Lock()
	if s.pq.Len() == 0 {
		s.mu.Unlock()
		return nil, false
	}

	// Pop the earliest event from our priority heap
	event := heap.Pop(&s.pq).(*Event)
	
	// Advance the virtual simulation clock straight to this event's execution time
	s.simClock = event.Timestamp
	s.mu.Unlock()

	// Inside our actual engine, this is where we will route the data to its destination handler:
	// e.g., router.HandleReceive(event.Data) if type is EventPacketRx
	
	return event, true
}

// CurrentTime returns the current virtual simulation clock time.
func (s *Scheduler) CurrentTime() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.simClock
}