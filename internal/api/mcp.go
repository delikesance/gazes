package api

import (
	"github.com/gazes/gazes/internal/admin"
	"github.com/gazes/gazes/internal/mcp"
	"github.com/go-chi/chi/v5"
)

// WithMCP mounts the MCP server (Streamable HTTP) at /mcp. It needs WithAdmin, authenticates by
// bearer token only, and refuses requests whose Origin header is not in allowedOrigins (none by
// default: MCP HTTP clients send no Origin).
func WithMCP(allowedOrigins ...string) Option {
	return func(s *Server) { s.mcpEnabled, s.mcpOrigins = true, allowedOrigins }
}

func (s *Server) mountMCP(r chi.Router) {
	if !s.mcpEnabled || s.admin == nil {
		return
	}
	// Tools call the admin API in process, through a private router that carries only the admin routes.
	s.admin.SetToolCatalogue(func() []admin.ToolInfo {
		defs := mcp.DefaultTools()
		out := make([]admin.ToolInfo, 0, len(defs))
		for _, d := range defs {
			out = append(out, admin.ToolInfo{Name: d.Name, Description: d.Description, Level: string(d.Level),
				Scope: d.Scope, Method: d.Method, Path: d.Path, Summary: d.Summary})
		}
		return out
	})
	inner := chi.NewRouter()
	s.admin.Mount(inner)
	h := mcp.New(mcp.Config{
		Admin:          inner,
		Tokens:         s.admin.Store(),
		Audit:          s.admin.Store(),
		AllowedOrigins: s.mcpOrigins,
	}, nil).HTTPHandler()
	r.Handle("/mcp", h)
}
