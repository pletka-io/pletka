package mcp

import (
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// serverVersion is reported to MCP clients; not tied to build info in v1.
const serverVersion = "0.1.0"

// newServer builds the MCP server and registers all read-only tools.
func newServer(h Host) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "pletka", Version: serverVersion}, nil)
	registerProjectTools(s, h)  // Task 8
	registerEntityTools(s, h)   // Task 9
	registerSemanticTools(s, h) // Task 10
	return s
}

// registerSemanticTools attaches semantic-surface tools. Stub until Task 10.
func registerSemanticTools(s *sdk.Server, h Host) {}
