package mcp

import (
	"strconv"
)

// Level is the autonomy level of a tool (docs: MCP design, "garde-fous").
type Level string

const (
	LevelRead       Level = "read"
	LevelDiagnostic Level = "diagnostic"
	LevelReversible Level = "reversible"
	LevelSensitive  Level = "sensitive"
)

// ReadOnly reports whether tools of this level never change state.
func (l Level) ReadOnly() bool { return l == LevelRead || l == LevelDiagnostic }

// Param describes one tool argument and where it goes in the admin request.
type Param struct {
	Name        string
	Type        string // "string" (default), "integer", "number" or "boolean"
	Description string
	Required    bool
	In          string   // "query" (default), "path" (replaces {Name} in ToolDef.Path), "body" (top-level JSON body field) or "arg" (field of the body's "args" object)
	Enum        []string // allowed values, as text (converted to the param type in the schema)
	Sensitive   bool     // never written to the audit log
}

// ToolDef declares a tool. Executing it calls the admin API in process with the caller's token,
// so authentication, scopes, bounds and privacy rules are exactly those of the HTTP API.
type ToolDef struct {
	Name        string
	Description string
	Level       Level
	Scope       string // admin token scope required (also enforced by the admin route itself)
	Method      string // HTTP method of the admin route
	Path        string // route relative to /api/v1/admin, e.g. "/overview" or "/issues/{id}"
	Params      []Param
	Summary     string // one line for the panel's tool catalogue
}

// DefaultTools is the tool table. Adding a tool = adding a ToolDef line here (see README.md).
func DefaultTools() []ToolDef {
	tools := []ToolDef{
		{
			Name: "get_overview", Level: LevelRead, Scope: "metrics:read", Method: "GET", Path: "/overview",
			Description: "Headline KPIs (sessions, watch time, active and new users, completion) for a period, with the change against the previous period.",
			Summary:     "KPIs and variations",
			Params:      []Param{{Name: "period", Type: "integer", Description: "Window in days.", Enum: []string{"7", "30", "90"}}},
		},
		{
			Name: "get_me", Level: LevelRead, Scope: "metrics:read", Method: "GET", Path: "/me",
			Description: "Describes the credential used for this connection: how it authenticates and which scopes it holds.",
			Summary:     "Current credential and scopes",
		},
	}
	tools = append(tools, readTools()...)
	tools = append(tools, actionTools()...)
	return tools
}

func (p Param) typ() string {
	if p.Type == "" {
		return "string"
	}
	return p.Type
}

// inputSchema is the JSON schema of the tool arguments.
func (d ToolDef) inputSchema() map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range d.Params {
		sch := map[string]any{"type": p.typ()}
		if p.Description != "" {
			sch["description"] = p.Description
		}
		if len(p.Enum) > 0 {
			vals := make([]any, 0, len(p.Enum))
			for _, e := range p.Enum {
				switch p.typ() {
				case "integer":
					if n, err := strconv.Atoi(e); err == nil {
						vals = append(vals, n)
					}
				case "boolean":
					if b, err := strconv.ParseBool(e); err == nil {
						vals = append(vals, b)
					}
				default:
					vals = append(vals, e)
				}
			}
			sch["enum"] = vals
		}
		props[p.Name] = sch
		if p.Required {
			required = append(required, p.Name)
		}
	}
	out := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}
