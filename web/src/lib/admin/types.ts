// Admin API response types derived from internal/admin/api_*.go

// Shared envelope wrapper for all API responses
export interface AdminEnvelope<T> {
  generated_at: string;
  period?: {
    days: number;
    from: string;
    to: string;
    prev_from: string;
    prev_to: string;
  };
  data: T;
}

// Shared KPI structure: value, previous period, and percentage delta
export interface Kpi {
  value: number;
  previous: number;
  /** null when the previous value is 0 (no meaningful ratio). */
  delta_pct: number | null;
}

// GET /me - Authentication info (from api.go handleMe)
export interface AdminMe {
  via: "token" | "session";
  role?: string; // "admin" for sessions only
  scopes: string[];
}

// GET /overview - KPI cards, daily series, top anime, heatmap (from api_views.go handleOverview)
export interface AdminOverview {
  kpis: {
    total_users: Kpi;
    new_users: Kpi;
    sessions: Kpi;
    watch_hours: Kpi;
    active_users_avg: Kpi;
    completion_pct: Kpi;
  };
  series: Array<{
    day: string;
    sessions: number;
    new_users: number;
    active_users: number;
  }>;
  top_anime: Array<{
    anime_id: number;
    title: string;
    sessions: number;
    share_pct: number;
    delta_pct: number | null;
  }>;
  heatmap: {
    weekday_start: "monday";
    hours_tz: "UTC";
    weekdays: string[];
    normalized: number[][];
    raw: number[][];
  };
  peak: {
    weekday: number;
    weekday_name: string;
    hour: number;
    sessions: number;
  } | null;
}

// GET /views - Session statistics, retention, drop episodes (from api_views.go handleViews)
export interface AdminViews {
  sessions_series: Array<{
    day: string;
    sessions: number;
    prev_day: string;
    prev_sessions: number;
  }>;
  avg_session_minutes: number;
  session_length_buckets: Array<{
    range_minutes: string;
    sessions: number;
    share_pct: number;
  }>;
  completion_by_weekday: Array<{
    weekday: number;
    name: string;
    sessions: number;
    completed: number;
    completion_pct: number;
  }>;
  sessions_by_hour: Array<{
    hour: number;
    sessions: number;
  }>;
  by_timezone: Array<{
    tz_offset: number;
    sessions: number;
    share_pct: number;
  }>;
  episodes_per_active_user: Kpi;
  resume_vs_first: {
    resume: number;
    first: number;
    resume_pct: number;
  };
  retention_curve: Array<{
    decile: number;
    from_pct: number;
    present: number;
    eligible: number;
    retained_pct: number;
  }>;
  drop_points: Array<{
    from_decile: number;
    to_decile: number;
    drop_pts: number;
  }>;
  drop_episodes: Array<{
    anime_id: number;
    title: string;
    season_id: number;
    episode: number;
    sessions: number;
    abandon_pct: number;
    median_drop_minute: number;
  }>;
}

// GET /catalog - Paginated anime table, quadrant, genres, languages (from api_views.go handleCatalog)
export interface AdminCatalog {
  format: "tv" | "movie" | "ova" | "all";
  anime: Array<{
    anime_id: number;
    title: string;
    format: string;
    sessions: number;
    watch_hours: number;
    completion_pct: number;
    new_viewers: number;
    delta_pct: number | null;
  }>;
  anime_total: number;
  limit: number;
  offset: number;
  quadrant: {
    median_sessions: number;
    median_completion_pct: number;
    points: Array<{
      anime_id: number;
      title: string;
      sessions: number;
      completion_pct: number;
      quadrant: string;
      quadrant_key: "safe" | "push" | "watch" | "retire";
    }>;
  };
  genres: Array<{
    genre: string;
    sessions: number;
    share_pct: number;
    completion_pct: number;
  }>;
  formats: Array<{
    key: string;
    sessions: number;
    share_pct: number;
  }>;
  audio_langs: Array<{
    key: string;
    sessions: number;
    share_pct: number;
  }>;
  sub_langs: Array<{
    key: string;
    sessions: number;
    share_pct: number;
  }>;
  new_vs_catalog: {
    new_series: {
      series: number;
      sessions: number;
      share_pct: number;
    };
    catalog: {
      series: number;
      sessions: number;
      share_pct: number;
    };
    history_from: string;
  };
}

// GET /users/summary - User KPIs, segments, seniority (from api_users.go handleUsersSummary)
export interface AdminUsersSummary {
  kpis: {
    total: Kpi;
    active_7d: Kpi;
    active_30d: Kpi;
    dormant: Kpi;
    no_session: Kpi;
  };
  segments: Array<{
    key: string;
    label: string;
    users: number;
    share_pct: number;
    avg_watch_hours: number;
  }>;
  seniority: Array<{
    key: string;
    label: string;
    users: number;
    share_pct: number;
  }>;
  active_sessions: {
    valid: number;
    expiring_7d: number;
  };
}

// GET /users - User list with pagination (from api_users.go handleUsersList)
export type AdminUserSegment =
  | "new"
  | "regular"
  | "power"
  | "dormant"
  | "at_risk"
  | "never_watched";

export interface AdminUserRow {
  user_id: number;
  /** Present for admin sessions only; the key is absent for API tokens. */
  pseudo?: string;
  created_at: string;
  last_activity: string | null;
  sessions: number;
  watch_hours: number;
  top_anime: {
    anime_id: number;
    title: string;
    watch_hours: number;
  } | null;
  segment: AdminUserSegment;
}

export interface AdminUsersList {
  total: number;
  limit: number;
  offset: number;
  users: AdminUserRow[];
}

// GET /users/{id} - User detail (from api_users.go handleUserDetail)
export interface AdminUserDetail {
  user_id: number;
  /** Present for admin sessions only; the key is absent for API tokens. */
  pseudo?: string;
  created_at: string;
  segment: AdminUserSegment;
  recent_sessions: Array<{
    anime_id: number;
    title: string;
    episode: number;
    started_at: string;
    watched_seconds: number;
    duration: number;
    completed: boolean;
  }>;
  progress: Array<{
    anime_id: number;
    season_id: number;
    title: string;
    episode: number;
    position: number;
    updated_at: string;
  }>;
}

// GET /growth - Growth metrics: DAU/WAU/MAU, cohorts, funnel (from api_users.go handleGrowth)
export interface AdminGrowth {
  activity: {
    dau: Kpi;
    wau: Kpi;
    mau: Kpi;
    stickiness_pct: Kpi;
  };
  activity_series: Array<{
    day: string;
    dau: number;
    wau: number;
    mau: number;
  }>;
  signups_per_week: Array<{
    week_start: string;
    signups: number;
  }>;
  cumulative_users: Array<{
    day: string;
    users: number;
  }>;
  funnel: {
    cohort_size: number;
    steps: Array<{
      key: string;
      label: string;
      users: number;
      eligible: number;
      excluded_too_recent: number;
      conversion_pct: number | null;
    }>;
    note: string;
  };
  cohorts: Array<{
    week_start: string;
    users: number;
    d1: number | null;
    d7: number | null;
    d14: number | null;
    d30: number | null;
  }>;
  time_to_first_session: {
    cohort_size: number;
    buckets: Array<{
      key: string;
      label: string;
      users: number;
      share_pct: number;
    }>;
  };
  churn: {
    previous_window_active: number;
    churned: number;
    churn_pct: number | null;
    window_days: number;
  };
  not_measured: string[];
  visitor_to_signup: {
    measured: false;
    value: null;
  };
  acquisition_sources: {
    measured: false;
    value: null;
  };
}

// Body of the cache diagnostics (internal/api/cache_handlers.go cacheDiagnostics).
// Without Redis only { redis: "disabled" } is returned.
export interface AdminCacheDiagnostics {
  redis:
    | "disabled"
    | { status: "ok" | "unreachable"; latency_ms?: number; keys?: number };
  stats?: Record<string, unknown>;
  anilist_cooldown_ms?: number;
}

// GET /playback/health - Player health metrics (from api_playback.go handlePlaybackHealth)
export interface AdminPlaybackHealth {
  active_sessions: {
    value: number | null;
    measured: boolean;
  };
  error_rate: {
    value: number | null;
    previous: number | null;
    delta_pct: number | null;
    errors: number;
    sessions: number;
    measured: boolean;
    note: string;
  };
  startup_ms: {
    p50: number | null;
    p95: number | null;
    measured: boolean;
  };
  sources: {
    active: number | null;
    total: number | null;
    failing: number;
    measured: boolean;
  };
  cache: {
    /** Cache diagnostics (null when not wired). */
    data: AdminCacheDiagnostics | null;
    measured: boolean;
  };
}

// GET /playback/errors - Paginated error list (from api_playback.go handlePlaybackErrors)
export interface AdminPlaybackErrorItem {
  code: string;
  anime_id: number | null;
  episode: number | null;
  source: string | null;
  occurrences: number;
  first_seen: string;
  last_seen: string;
}

export interface AdminPlaybackErrors {
  since: string;
  items: AdminPlaybackErrorItem[];
  page: {
    limit: number;
    offset: number;
    total: number;
  };
}

// GET /playback/errors/summary - Error codes with daily trend (from api_playback.go handlePlaybackErrorsSummary)
export interface AdminErrorsSummary {
  total: Kpi;
  by_code: Array<{
    code: string;
    count: Kpi;
  }>;
  daily: Array<{
    day: string;
    total: number;
    by_code: { [code: string]: number };
  }>;
  probable_causes: Array<{
    code: string;
    count: number;
    probable_cause: string;
    files: string[];
  }>;
}

// GET /playback/sources - Failure counts per source (from api_playback.go handlePlaybackSources)
export interface AdminSources {
  sources: Array<{
    source: string;
    failures: Kpi;
    share_pct: number;
    last_seen: string;
    failure_rate: null;
    measured: boolean;
  }>;
  total_failures: number;
  not_measured: string[];
}

// GET /costs - Usage and costs breakdown (from api_playback_costs.go handleCosts)
export interface AdminCosts {
  usage: {
    watch_hours: Kpi;
    sessions: Kpi;
    active_users: Kpi;
    peak_concurrent_sessions: {
      value: number | null;
      estimated: boolean;
      truncated: boolean;
      method: string;
    };
  };
  costs: {
    currency_unit: string;
    server: {
      value: number | null;
      measured: boolean;
      missing_inputs?: string[];
    };
    bandwidth: {
      value: number | null;
      measured: boolean;
      missing_inputs?: string[];
    };
    storage: {
      value: number | null;
      measured: boolean;
      missing_inputs?: string[];
    };
    total: {
      value: number | null;
      measured: boolean;
      partial?: boolean;
    };
    per_watch_hour: {
      value: number | null;
      measured: boolean;
    };
    per_active_user: {
      value: number | null;
      measured: boolean;
    };
    server_prorata: number;
  };
}

// GET /issues - Issue list with pagination (from api_playback_issues.go handleIssuesList)
export interface AdminIssue {
  id: string;
  created_at: string;
  updated_at: string;
  severity: "low" | "medium" | "high" | "critical";
  status: "new" | "in_progress" | "resolved";
  title: string;
  evidence: string | null;
  suggested_fix: string | null;
  source: string | null;
  note: string | null;
}

export interface AdminIssuesList {
  items: AdminIssue[];
  page: {
    limit: number;
    offset: number;
    total: number;
  };
}

// GET /issues/{id} - Issue detail response is AdminIssue

// POST /issues - Create issue request
export interface AdminIssueCreate {
  title: string;
  severity: "low" | "medium" | "high" | "critical";
  evidence?: string;
  suggested_fix?: string;
  source?: string;
}

// PATCH /issues/{id} - Update issue request
export interface AdminIssueUpdate {
  status?: "new" | "in_progress" | "resolved";
  note?: string;
}

// ---- Claude et MCP / Paramètres (internal/admin/api_ops.go, api_settings.go, ops.go) ----

export type AdminApprovalStatus = "pending" | "approved" | "executed" | "rejected" | "failed" | "undone";

// GET /approvals/{id} and the items of GET /approvals (api_ops.go approvalView)
export interface AdminApproval {
  id: number;
  created_at: string;
  tool: string;
  args: Record<string, unknown>;
  justification: string | null;
  expected_effect: string | null;
  plan: { summary: string; details: Record<string, unknown> } | null;
  status: AdminApprovalStatus;
  requested_by_token: number | null;
  decided_by: string | null;
  decided_at: string | null;
  executed_at: string | null;
  result: Record<string, unknown> | null;
  undoable: boolean;
}

// GET /approvals?status=&limit=&offset=
export interface AdminApprovalsList {
  items: AdminApproval[];
  page: { limit: number; offset: number; total: number };
}

// GET /ops/kill-switch (ops.go KillSwitch)
export interface AdminKillSwitch {
  suspended: boolean;
  reason: string;
  updated_at: string | null;
  updated_by: string | null;
}

export type AdminActionLevel = "reversible" | "sensitive";

// GET /ops/actions
export interface AdminAction {
  name: string;
  level: AdminActionLevel;
  scope: string;
  summary: string;
  implemented: boolean;
}
export interface AdminActions {
  items: AdminAction[];
  reversible_budget_per_hour: number;
}

export type AdminToolLevel = "read" | "diagnostic" | "reversible" | "sensitive";

// GET /mcp/tools (api_settings.go ToolInfo; empty and enabled:false when /mcp is not mounted)
export interface AdminMcpTool {
  name: string;
  description: string;
  level: AdminToolLevel;
  scope: string;
  method: string;
  path: string;
  summary: string;
}
export interface AdminMcpTools {
  enabled: boolean;
  items: AdminMcpTool[];
}

// GET /mcp/audit?outcome=&limit=&offset= (arguments were redacted and truncated when written)
export interface AdminMcpAuditRow {
  id: number;
  ts: string;
  token_id: number | null;
  tool: string;
  args_summary: string;
  outcome: string;
  duration_ms: number;
  approval_id: number | null;
}
export interface AdminMcpAudit {
  total: number;
  limit: number;
  offset: number;
  items: AdminMcpAuditRow[];
}

// GET /settings
export interface AdminThreshold {
  rule: string;
  value: number;
  default: number;
  min: number;
  max: number;
}
export interface AdminSettings {
  thresholds: AdminThreshold[];
  reversible_budget_per_hour: number;
  kill_switch: AdminKillSwitch;
}

// GET /tokens (session only; metadata, never a hash or a secret)
export interface AdminToken {
  id: number;
  name: string;
  scopes: string[];
  created_at: string;
  expires_at: string;
  last_used_at: string | null;
  status: string;
}
export interface AdminTokens {
  items: AdminToken[];
}
