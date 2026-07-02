package protocol

// RouteProtocol identifies how a route entered the routing table.
type RouteProtocol string

const (
	RouteConnected RouteProtocol = "C"
	RouteStatic    RouteProtocol = "S"
	RouteOSPF      RouteProtocol = "O"
	RouteRIP       RouteProtocol = "R"
)

// Default admin distances used for route selection.
const (
	AdminDistConnected = 0
	AdminDistStatic    = 1
	AdminDistRIP       = 120
	AdminDistOSPF      = 110
)
