package mcp

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gazes/gazes/internal/admin"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HTTPHandler returns the Streamable HTTP endpoint (mount it at /mcp). Requests are accepted only
// with "Authorization: Bearer gzs_..."; cookies are never read. A request carrying an Origin header
// outside Config.AllowedOrigins is refused (DNS rebinding protection; MCP HTTP clients send none).
func (s *Server) HTTPHandler() http.Handler {
	inner := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.sdk },
		&mcp.StreamableHTTPOptions{JSONResponse: true})
	authed := auth.RequireBearerToken(s.verifyBearer, nil)(inner)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && !s.originAllowed(o) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		authed.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin string) bool {
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	for _, a := range s.cfg.AllowedOrigins {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), origin) {
			return true
		}
	}
	return false
}

// verifyBearer checks the token like the admin API does (same store: unknown, expired and revoked
// tokens are all refused) and carries its identity into the request.
func (s *Server) verifyBearer(ctx context.Context, plain string, _ *http.Request) (*auth.TokenInfo, error) {
	tok, err := s.cfg.Tokens.VerifyToken(ctx, plain)
	if err != nil {
		if errors.Is(err, admin.ErrInvalidToken) {
			return nil, auth.ErrInvalidToken
		}
		return nil, errors.New("token verification unavailable")
	}
	return tokenInfo(tok, plain), nil
}

func tokenInfo(tok *admin.Token, plain string) *auth.TokenInfo {
	return &auth.TokenInfo{
		Scopes:     tok.Scopes,
		Expiration: tok.ExpiresAt,
		UserID:     strconv.FormatInt(tok.ID, 10), // binds an MCP session to its token
		Extra:      map[string]any{"bearer": plain},
	}
}

// VerifyStatic verifies plain once and returns the credential for a stdio connection.
func VerifyStatic(ctx context.Context, v admin.TokenVerifier, plain string) (*Credential, error) {
	tok, err := v.VerifyToken(ctx, plain)
	if err != nil {
		return nil, err
	}
	return &Credential{TokenID: tok.ID, Plain: plain, Scopes: tok.Scopes}, nil
}
