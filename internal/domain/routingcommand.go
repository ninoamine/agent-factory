// Package domain holds the core types. No I/O, no provider code.
package domain

// RoutingCommand selects the model for one request (ROUTE-1).
type RoutingCommand string

const (
	CmdFast     RoutingCommand = "/fast"
	CmdCode     RoutingCommand = "/code"
	CmdReason   RoutingCommand = "/reason"
	CmdCreative RoutingCommand = "/creative"
)

// RoutingCommands is the fixed list, in display order. Not extensible.
var RoutingCommands = []RoutingCommand{CmdFast, CmdCode, CmdReason, CmdCreative}

// ToolCommands control the app and are never routed (ROUTE-7).
var ToolCommands = []string{"/refresh", "/suggest", "/models", "/history", "/help"}

// ModelRef identifies a model by its id in models.json.
type ModelRef string
