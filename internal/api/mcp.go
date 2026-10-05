package api

import (
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
