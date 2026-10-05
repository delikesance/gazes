// Package mcp is the Gazes admin MCP server. Tools are declarative (ToolDef) and run by calling
// the admin HTTP API in process with the caller's own bearer token, so scopes, bounds and privacy
// rules are those of the API and are never re-implemented here.
package mcp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gazes/gazes/internal/admin"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the server version announced to clients.
const Version = "0.1.0"

// AuditSink records tool calls (implemented by *admin.Store). Failures never fail a call.
type AuditSink interface {
	RecordMCPAudit(ctx context.Context, e admin.MCPAuditEntry) error
}

// Config configures a Server.
type Config struct {
	Admin          http.Handler        // serves /api/v1/admin/* (the admin Service mounted on a router)
	Tokens         admin.TokenVerifier // verifies bearer tokens (the admin Store)
	Audit          AuditSink           // optional
	Tools          []ToolDef           // defaults to DefaultTools()
	AllowedOrigins []string            // Origin values accepted on HTTP; empty = any request carrying Origin is refused
	Logger         *slog.Logger
	Now            func() time.Time
}

// Credential is the authenticated caller of a tool call.
type Credential struct {
	TokenID int64
	Plain   string // the bearer token, forwarded to the admin router
	Scopes  []string
}

func (c *Credential) has(scope string) bool {
	if c == nil {
		return false
	}
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Server is the MCP server shared by the HTTP and stdio transports.
type Server struct {
	cfg    Config
	sdk    *mcp.Server
	tools  map[string]ToolDef
	static *Credential // stdio / in-memory: one credential for the whole connection
}

// New builds a Server. static is the credential for transports without per-request
// authentication (stdio); HTTP ignores it and authenticates every request.
func New(cfg Config, static *Credential) *Server {
	if cfg.Tools == nil {
		cfg.Tools = DefaultTools()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Server{cfg: cfg, tools: map[string]ToolDef{}, static: static}
	s.sdk = mcp.NewServer(&mcp.Implementation{Name: "gazes-admin", Version: Version}, nil)
	for _, d := range cfg.Tools {
		s.tools[d.Name] = d
		s.sdk.AddTool(&mcp.Tool{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.inputSchema(),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: d.Level.ReadOnly()},
		}, s.handler(d))
	}
	s.sdk.AddReceivingMiddleware(s.filterTools)
	s.AddResourcesAndPrompts()
	return s
}

// SDK exposes the underlying SDK server (tests, custom transports).
func (s *Server) SDK() *mcp.Server { return s.sdk }

// RunStdio serves the protocol on stdin/stdout until ctx ends or the client disconnects.
func (s *Server) RunStdio(ctx context.Context) error {
	return s.sdk.Run(ctx, &mcp.StdioTransport{})
}

// credential extracts the caller from a request: the verified HTTP token, else the static one.
func (s *Server) credential(extra *mcp.RequestExtra) *Credential {
	if extra != nil && extra.TokenInfo != nil {
		ti := extra.TokenInfo
		plain, _ := ti.Extra["bearer"].(string)
		id, _ := strconv.ParseInt(ti.UserID, 10, 64)
		return &Credential{TokenID: id, Plain: plain, Scopes: ti.Scopes}
	}
	return s.static
}

// filterTools hides, in tools/list, the tools whose scope the caller lacks.
func (s *Server) filterTools(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)
		if err != nil || method != "tools/list" {
			return res, err
		}
		lr, ok := res.(*mcp.ListToolsResult)
		if !ok {
			return res, err
		}
		cred := s.credential(req.GetExtra())
		out := make([]*mcp.Tool, 0, len(lr.Tools))
		for _, t := range lr.Tools {
			if d, ok := s.tools[t.Name]; ok && cred.has(d.Scope) {
				out = append(out, t)
			}
		}
		lr.Tools = out
		return lr, nil
	}
}

func (s *Server) handler(d ToolDef) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := s.cfg.Now()
		cred := s.credential(req.Extra)
		var args map[string]any
		if len(req.Params.Arguments) > 0 {
			_ = json.Unmarshal(req.Params.Arguments, &args)
		}
		text, outcome, terr := s.run(ctx, d, cred, args)
		s.audit(ctx, cred, d, args, outcome, s.cfg.Now().Sub(start))
		if terr != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: terr.Error()}}}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	}
}

// run executes one tool call and classifies it (ok, error or denied).
func (s *Server) run(ctx context.Context, d ToolDef, cred *Credential, args map[string]any) (string, string, error) {
	if cred == nil || cred.Plain == "" || !cred.has(d.Scope) {
		return "", "denied", &toolError{msg: "denied: missing scope " + d.Scope}
	}
	req, err := buildRequest(ctx, d, args, cred.Plain)
	if err != nil {
		return "", "error", err
	}
	status, body, err := serveAdmin(s.cfg.Admin, req)
	if err != nil {
		return "", "error", err
	}
	text, err := shapeResponse(status, body)
	if err != nil {
		if te, ok := err.(*toolError); ok && te.denied {
			return "", "denied", err
		}
		return "", "error", err
	}
	return text, "ok", nil
}

// audit records the call; it never fails or delays the call beyond a short timeout.
func (s *Server) audit(ctx context.Context, cred *Credential, d ToolDef, args map[string]any, outcome string, dur time.Duration) {
	if s.cfg.Audit == nil {
		return
	}
	var id int64
	if cred != nil {
		id = cred.TokenID
	}
	actx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	defer func() { _ = recover() }()
	err := s.cfg.Audit.RecordMCPAudit(actx, admin.MCPAuditEntry{
		TS: s.cfg.Now(), TokenID: id, Tool: d.Name, ArgsSummary: argsSummary(d, args),
		Outcome: outcome, DurationMS: dur.Milliseconds(),
	})
	if err != nil {
		s.cfg.Logger.Warn("mcp audit write failed", "tool", d.Name, "err", err)
	}
}

// AddResourcesAndPrompts adds the read-only resources and guidance prompts to the server.
func (s *Server) AddResourcesAndPrompts() {
	// Resources: callable with resources/read if the token has the required scope
	s.sdk.AddResource(&mcp.Resource{
		URI:      "gazes://overview",
		Name:     "Overview (30d)",
		MIMEType: "application/json",
	}, s.resourceHandler("get_overview", 30))

	s.sdk.AddResource(&mcp.Resource{
		URI:      "gazes://issues/open",
		Name:     "Open issues",
		MIMEType: "application/json",
	}, s.resourceHandler("list_issues", 0))

	s.sdk.AddResource(&mcp.Resource{
		URI:      "gazes://health/player",
		Name:     "Player health (7d)",
		MIMEType: "application/json",
	}, s.resourceHandler("get_player_health", 7))

	s.sdk.AddResource(&mcp.Resource{
		URI:      "gazes://costs/month",
		Name:     "Costs (30d)",
		MIMEType: "application/json",
	}, s.resourceHandler("get_costs", 30))

	// Prompts: guidance for Claude on which tools to call and what conclusions to draw
	s.sdk.AddPrompt(&mcp.Prompt{
		Name:        "weekly-review",
		Description: "Review platform health and user metrics for the past week",
	}, s.promptHandler("weekly-review"))

	s.sdk.AddPrompt(&mcp.Prompt{
		Name:        "incident-triage",
		Description: "Triage a playback incident: diagnose the error, identify probable cause and affected users",
	}, s.promptHandler("incident-triage"))

	s.sdk.AddPrompt(&mcp.Prompt{
		Name:        "cost-audit",
		Description: "Audit platform costs: compare with budget, identify cost drivers, propose optimizations",
	}, s.promptHandler("cost-audit"))

	s.sdk.AddPrompt(&mcp.Prompt{
		Name:        "business-plan-12m",
		Description: "Plan the next 12 months: growth targets, user segments to focus on, retention strategies",
	}, s.promptHandler("business-plan-12m"))

	s.sdk.AddPrompt(&mcp.Prompt{
		Name:        "activation-review",
		Description: "Review user activation funnel: analyze signup→first session→retention, identify drop-offs",
	}, s.promptHandler("activation-review"))
}

// resourceHandler creates a handler for a resource URI that calls a tool or list_issues.
func (s *Server) resourceHandler(toolName string, periodDays int) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		cred := s.credential(req.Extra)
		toolDef, ok := s.tools[toolName]
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		if !cred.has(toolDef.Scope) {
			return nil, mcp.ResourceNotFoundError(req.Params.URI) // hidden, like tools/list: do not reveal what exists
		}

		var args map[string]any
		if toolName == "list_issues" {
			args = map[string]any{"limit": 200, "offset": 0}
		} else if periodDays > 0 {
			args = map[string]any{"period": periodDays}
		}

		text, outcome, terr := s.run(ctx, toolDef, cred, args)
		s.audit(ctx, cred, toolDef, args, outcome, 0)
		if terr != nil {
			return nil, terr
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{
			{URI: req.Params.URI, MIMEType: "application/json", Text: text},
		}}, nil
	}
}

// promptHandler creates a handler for a prompt that returns guidance text.
func (s *Server) promptHandler(promptName string) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		cred := s.credential(req.Extra)
		text := promptText(promptName, cred)
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: text}},
		}}, nil
	}
}

// promptText returns the guidance text for a prompt name.
func promptText(name string, cred *Credential) string {
	switch name {
	case "weekly-review":
		return `Review the platform's health and user metrics for the past week.

Call in order:
1. get_overview(7) — headline KPIs and variations
2. get_growth(7) — DAU/WAU/MAU, new signups, funnel conversion
3. get_player_health(7) — error rate and playback health
4. list_issues(limit=10, status="new") — recent issues raised

Conclude with: summary of key metrics (numbers, not null guesses), health status (green/yellow/red), action items with measurable targets.`

	case "incident-triage":
		return `Diagnose a playback incident.

Call in order:
1. get_errors_summary(7) — error distribution, probable causes
2. list_playback_errors(limit=50) — recent grouped errors
3. get_sources_health(7) — source/tracker status
4. get_player_health(7) — error rate and active sessions

Conclude with: root cause (code or source), probable file to inspect (from causes list), affected users (estimated), recovery steps (if reversible).`

	case "cost-audit":
		return `Audit platform costs and identify drivers.

Call in order:
1. get_costs(30) — usage and cost breakdown (only measured items)
2. get_overview(30) — sessions and watch hours
3. list_top_anime(30, limit=20) — content distribution
4. get_growth(30) — user growth and retention

Conclude with: total cost and per-unit metrics (only measured), cost drivers ranked, 3 optimization proposals with projected % savings, caveats (missing cost inputs, storage unmeasured).`

	case "business-plan-12m":
		return `Plan the next 12 months: growth targets and retention strategy.

Call in order:
1. get_growth(90) — DAU/WAU/MAU, stickiness, churn, cohort retention
2. get_overview(90) — sessions, watch hours, completion
3. get_users_summary() — user segments
4. get_retention() — J1/J7/J14/J30 retention by cohort

Conclude with: 12-month targets (DAU, retention, churn %), per-segment strategy (new, power, at-risk), retention levers to pull (measured by J30 %), checkpoints at months 3/6/9.`

	case "activation-review":
		return `Analyze the signup→retention funnel and identify drop-offs.

Call in order:
1. get_growth(30) — funnel (signup→first→3 episodes→J7→J30) and cohorts
2. get_retention() — cohort retention by signup week
3. get_users_summary() — segment distribution
4. list_users(segment="new", limit=20) — newest users

Conclude with: funnel drop-off stages (%) and metrics, J1/J7/J30 target vs actual, drop-off root causes (from user data or playback health), 3 activation levers with expected impact (measured by J30 %).`

	default:
		return "Unknown prompt: " + name
	}
}
