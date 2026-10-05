// Admin API fixtures: realistic sample data for development and testing
import type {
  AdminMe,
  AdminOverview,
  AdminViews,
  AdminCatalog,
  AdminUsersSummary,
  AdminUsersList,
  AdminUserDetail,
  AdminGrowth,
  AdminPlaybackHealth,
  AdminPlaybackErrors,
  AdminErrorsSummary,
  AdminSources,
  AdminCosts,
  AdminIssuesList,
  AdminIssue,
  Kpi,
} from "./types";

function dateRange(startDate: string, days: number): string[] {
  const dates: string[] = [];
  const date = new Date(startDate);
  for (let i = 0; i < days; i++) {
    dates.push(date.toISOString().split("T")[0]);
    date.setDate(date.getDate() + 1);
  }
  return dates;
}

const baseDates = dateRange("2026-09-05", 30);
const prevDates = dateRange("2026-08-06", 30);

// KPI with delta
function kpi(value: number, previous: number): Kpi {
  const delta_pct = previous === 0 ? null : ((value - previous) / previous) * 100;
  return { value, previous, delta_pct };
}

export const fixtures = {
  me: {
    via: "session" as const,
    role: "admin",
    scopes: ["metrics:read", "diagnostics:read", "ops:write", "config:write"],
  } as AdminMe,

  overview: {
    kpis: {
      total_users: kpi(18420, 16890),
      new_users: kpi(1254, 1089),
      sessions: kpi(124300, 108420),
      watch_hours: kpi(47200, 39800),
      active_users_avg: kpi(8620, 7950),
      completion_pct: kpi(42.3, 38.9),
    },
    series: baseDates.map((day, i) => {
      const dayIdx = i % 30;
      return {
        day,
        sessions: 4143 + Math.sin(dayIdx * 0.2) * 1200,
        new_users: 42 + Math.sin(dayIdx * 0.15) * 15,
        active_users: 287 + Math.cos(dayIdx * 0.18) * 80,
      };
    }),
    top_anime: [
      { anime_id: 1, title: "Frieren: Beyond Journey's End", sessions: 12400, share_pct: 9.97, delta_pct: 12.3 },
      { anime_id: 2, title: "Solo Leveling", sessions: 11850, share_pct: 9.53, delta_pct: 8.7 },
      { anime_id: 3, title: "Dandadan", sessions: 10200, share_pct: 8.21, delta_pct: 15.2 },
      { anime_id: 4, title: "Jujutsu Kaisen", sessions: 9450, share_pct: 7.6, delta_pct: -3.1 },
      { anime_id: 5, title: "Kaiju No. 8", sessions: 8720, share_pct: 7.02, delta_pct: 6.8 },
      { anime_id: 6, title: "Blue Lock", sessions: 8200, share_pct: 6.59, delta_pct: -1.2 },
      { anime_id: 7, title: "Spy x Family", sessions: 7890, share_pct: 6.35, delta_pct: 2.1 },
    ],
    heatmap: {
      weekday_start: "monday" as const,
      hours_tz: "UTC",
      weekdays: ["lun", "mar", "mer", "jeu", "ven", "sam", "dim"],
      normalized: Array(7)
        .fill(null)
        .map((_, wd) =>
          Array(24)
            .fill(null)
            .map((_, h) => {
              const peak = (20 + Math.sin(wd * 0.8 + h * 0.1)) / 4;
              return Math.max(0.1, Math.min(1.0, 0.3 + peak * 0.7));
            })
        ),
      raw: Array(7)
        .fill(null)
        .map((_, wd) =>
          Array(24)
            .fill(null)
            .map((_, h) => {
              const baseHour = h >= 20 && h <= 23 ? 450 : h >= 10 && h <= 18 ? 280 : 100;
              return Math.round(baseHour + Math.sin(wd + h * 0.1) * 100);
            })
        ),
    },
    peak: { weekday: 5, weekday_name: "sam", hour: 22, sessions: 1240 },
  } as AdminOverview,

  views: {
    sessions_series: baseDates.map((day, i) => {
      const prevIdx = (i % 30) < prevDates.length ? i % 30 : prevDates.length - 1;
      return {
        day,
        sessions: 4143 + Math.sin(i * 0.2) * 1200,
        prev_day: prevDates[prevIdx],
        prev_sessions: 3620 + Math.sin(prevIdx * 0.2) * 1100,
      };
    }),
    avg_session_minutes: 22.8,
    session_length_buckets: [
      { range_minutes: "<5", sessions: 24100, share_pct: 19.4 },
      { range_minutes: "5-15", sessions: 38500, share_pct: 30.9 },
      { range_minutes: "15-30", sessions: 35200, share_pct: 28.3 },
      { range_minutes: "30-60", sessions: 18400, share_pct: 14.8 },
      { range_minutes: ">60", sessions: 8100, share_pct: 6.5 },
    ],
    completion_by_weekday: [
      { weekday: 0, name: "lun", sessions: 17200, completed: 7300, completion_pct: 42.4 },
      { weekday: 1, name: "mar", sessions: 18900, completed: 8100, completion_pct: 42.9 },
      { weekday: 2, name: "mer", sessions: 18100, completed: 7600, completion_pct: 42.0 },
      { weekday: 3, name: "jeu", sessions: 17800, completed: 7500, completion_pct: 42.1 },
      { weekday: 4, name: "ven", sessions: 19200, completed: 8300, completion_pct: 43.2 },
      { weekday: 5, name: "sam", sessions: 19800, completed: 8700, completion_pct: 43.9 },
      { weekday: 6, name: "dim", sessions: 18300, completed: 7800, completion_pct: 42.6 },
    ],
    sessions_by_hour: Array(24)
      .fill(null)
      .map((_, h) => ({
        hour: h,
        sessions: h >= 20 && h <= 23 ? 15000 : h >= 8 && h <= 18 ? 9200 : 3400,
      })),
    by_timezone: [
      { tz_offset: 60, sessions: 32100, share_pct: 25.8 },
      { tz_offset: 120, sessions: 28900, share_pct: 23.2 },
      { tz_offset: 0, sessions: 18400, share_pct: 14.8 },
      { tz_offset: -300, sessions: 12800, share_pct: 10.3 },
      { tz_offset: 330, sessions: 16200, share_pct: 13.0 },
      { tz_offset: -480, sessions: 9100, share_pct: 7.3 },
      { tz_offset: 540, sessions: 5800, share_pct: 4.7 },
      { tz_offset: 9000, sessions: 1000, share_pct: 0.8 },
    ],
    episodes_per_active_user: kpi(8.2, 7.1),
    resume_vs_first: { resume: 68400, first: 55900, resume_pct: 54.9 },
    retention_curve: [
      { decile: 0, from_pct: 0, present: 124300, eligible: 124300, retained_pct: 100.0 },
      { decile: 1, from_pct: 10, present: 89200, eligible: 124300, retained_pct: 71.7 },
      { decile: 2, from_pct: 20, present: 62400, eligible: 124300, retained_pct: 50.2 },
      { decile: 3, from_pct: 30, present: 44100, eligible: 124300, retained_pct: 35.5 },
      { decile: 4, from_pct: 40, present: 31200, eligible: 124300, retained_pct: 25.1 },
      { decile: 5, from_pct: 50, present: 22800, eligible: 124300, retained_pct: 18.3 },
      { decile: 6, from_pct: 60, present: 16400, eligible: 124300, retained_pct: 13.2 },
      { decile: 7, from_pct: 70, present: 11900, eligible: 124300, retained_pct: 9.6 },
      { decile: 8, from_pct: 80, present: 8200, eligible: 124300, retained_pct: 6.6 },
      { decile: 9, from_pct: 90, present: 4800, eligible: 124300, retained_pct: 3.9 },
    ],
    drop_points: [
      { from_decile: 0, to_decile: 1, drop_pts: 28.3 },
      { from_decile: 1, to_decile: 2, drop_pts: 21.5 },
      { from_decile: 2, to_decile: 3, drop_pts: 14.7 },
    ],
    drop_episodes: [
      {
        anime_id: 1,
        title: "Frieren: Beyond Journey's End",
        season_id: 1,
        episode: 5,
        sessions: 1240,
        abandon_pct: 18.2,
        median_drop_minute: 14.3,
      },
      {
        anime_id: 3,
        title: "Dandadan",
        season_id: 1,
        episode: 3,
        sessions: 980,
        abandon_pct: 22.1,
        median_drop_minute: 11.8,
      },
      {
        anime_id: 4,
        title: "Jujutsu Kaisen",
        season_id: 2,
        episode: 2,
        sessions: 850,
        abandon_pct: 15.9,
        median_drop_minute: 13.2,
      },
    ],
  } as AdminViews,

  catalog: {
    format: "all" as const,
    anime: [
      {
        anime_id: 1,
        title: "Frieren: Beyond Journey's End",
        format: "TV",
        sessions: 12400,
        watch_hours: 4960,
        completion_pct: 44.2,
        new_viewers: 340,
        delta_pct: 12.3,
      },
      {
        anime_id: 2,
        title: "Solo Leveling",
        format: "TV",
        sessions: 11850,
        watch_hours: 4200,
        completion_pct: 41.8,
        new_viewers: 320,
        delta_pct: 8.7,
      },
      {
        anime_id: 3,
        title: "Dandadan",
        format: "TV",
        sessions: 10200,
        watch_hours: 3600,
        completion_pct: 39.5,
        new_viewers: 280,
        delta_pct: 15.2,
      },
    ],
    anime_total: 3,
    limit: 50,
    offset: 0,
    quadrant: {
      median_sessions: 8500,
      median_completion_pct: 40.5,
      points: [
        {
          anime_id: 1,
          title: "Frieren: Beyond Journey's End",
          sessions: 12400,
          completion_pct: 44.2,
          quadrant: "valeurs sûres",
          quadrant_key: "safe" as const,
        },
        {
          anime_id: 2,
          title: "Solo Leveling",
          sessions: 11850,
          completion_pct: 41.8,
          quadrant: "valeurs sûres",
          quadrant_key: "safe" as const,
        },
      ],
    },
    genres: [
      { genre: "Action", sessions: 42100, share_pct: 33.9, completion_pct: 42.1 },
      { genre: "Drama", sessions: 38200, share_pct: 30.7, completion_pct: 41.8 },
      { genre: "Fantasy", sessions: 35900, share_pct: 28.9, completion_pct: 43.2 },
    ],
    formats: [
      { key: "tv", sessions: 98200, share_pct: 79.0 },
      { key: "movie", sessions: 18400, share_pct: 14.8 },
      { key: "ova", sessions: 7700, share_pct: 6.2 },
    ],
    audio_langs: [
      { key: "jp", sessions: 108900, share_pct: 87.6 },
      { key: "fr", sessions: 12200, share_pct: 9.8 },
      { key: "en", sessions: 3200, share_pct: 2.6 },
    ],
    sub_langs: [
      { key: "fr", sessions: 94200, share_pct: 75.8 },
      { key: "en", sessions: 25100, share_pct: 20.2 },
      { key: "es", sessions: 5000, share_pct: 4.0 },
    ],
    new_vs_catalog: {
      new_series: { series: 12, sessions: 31500, share_pct: 25.3 },
      catalog: { series: 245, sessions: 92800, share_pct: 74.7 },
      history_from: "2024-06-01",
    },
  } as AdminCatalog,

  usersSummary: {
    kpis: {
      total: kpi(18420, 16890),
      active_7d: kpi(9850, 8920),
      active_30d: kpi(14200, 12800),
      dormant: kpi(1240, 1450),
      no_session: kpi(820, 950),
    },
    segments: [
      { key: "new", label: "Nouveaux (< 7 j)", users: 340, share_pct: 1.85, avg_watch_hours: 2.1 },
      { key: "regular", label: "Réguliers", users: 11200, share_pct: 60.8, avg_watch_hours: 4.8 },
      { key: "power", label: "Power users (> 10 h/semaine)", users: 3400, share_pct: 18.5, avg_watch_hours: 14.2 },
      { key: "dormant", label: "Dormants (> 30 j)", users: 1240, share_pct: 6.7, avg_watch_hours: 0.3 },
      { key: "at_risk", label: "À risque (aucune séance depuis 14 j+)", users: 1800, share_pct: 9.8, avg_watch_hours: 0.8 },
      { key: "never_watched", label: "Jamais regardé", users: 440, share_pct: 2.4, avg_watch_hours: 0.0 },
    ],
    seniority: [
      { key: "lt_7d", label: "< 7 j", users: 340, share_pct: 1.85 },
      { key: "7_30d", label: "7-30 j", users: 890, share_pct: 4.83 },
      { key: "30_90d", label: "30-90 j", users: 1450, share_pct: 7.88 },
      { key: "90_180d", label: "90-180 j", users: 2100, share_pct: 11.41 },
      { key: "180_365d", label: "180-365 j", users: 3200, share_pct: 17.37 },
      { key: "gt_365d", label: "> 1 an", users: 10440, share_pct: 56.66 },
    ],
    active_sessions: { valid: 8240, expiring_7d: 420 },
  } as AdminUsersSummary,

  usersList: {
    total: 18420,
    limit: 50,
    offset: 0,
    users: [
      {
        user_id: 1001,
        pseudo: "AnimeFan42",
        created_at: "2025-02-14T10:30:00Z",
        last_activity: "2026-09-04T22:15:00Z",
        sessions: 342,
        watch_hours: 1248.5,
        segment: "power",
        top_anime: { anime_id: 1, title: "Frieren: Beyond Journey's End", watch_hours: 48.2 },
      },
      {
        user_id: 1002,
        pseudo: "TokyoNights",
        created_at: "2025-08-20T14:22:00Z",
        last_activity: "2026-09-03T18:45:00Z",
        sessions: 128,
        watch_hours: 420.3,
        segment: "regular",
        top_anime: { anime_id: 2, title: "Solo Leveling", watch_hours: 32.1 },
      },
      {
        user_id: 1003,
        created_at: "2026-08-28T09:15:00Z",
        last_activity: "2026-09-04T20:30:00Z",
        sessions: 18,
        watch_hours: 42.5,
        segment: "new",
        top_anime: { anime_id: 3, title: "Dandadan", watch_hours: 8.4 },
      },
    ],
  } as AdminUsersList,

  userDetail: {
    user_id: 1001,
    pseudo: "AnimeFan42",
    created_at: "2025-02-14T10:30:00Z",
    segment: "power",
    recent_sessions: [
      {
        anime_id: 1,
        title: "Frieren: Beyond Journey's End",
        episode: 12,
        started_at: "2026-09-04T22:00:00Z",
        watched_seconds: 1420,
        duration: 1400,
        completed: true,
      },
      {
        anime_id: 3,
        title: "Dandadan",
        episode: 8,
        started_at: "2026-09-04T20:30:00Z",
        watched_seconds: 1350,
        duration: 1400,
        completed: true,
      },
    ],
    progress: [
      {
        anime_id: 2,
        season_id: 1,
        title: "Solo Leveling",
        episode: 6,
        position: 840.5,
        updated_at: "2026-09-04T21:00:00Z",
      },
      {
        anime_id: 4,
        season_id: 2,
        title: "Jujutsu Kaisen",
        episode: 18,
        position: 600.0,
        updated_at: "2026-09-03T19:30:00Z",
      },
    ],
  } as AdminUserDetail,

  growth: {
    activity: {
      dau: kpi(8620, 7950),
      wau: kpi(12400, 11200),
      mau: kpi(14200, 12800),
      stickiness_pct: kpi(60.8, 62.1),
    },
    activity_series: baseDates.map((day, i) => ({
      day,
      dau: 8620 + Math.sin(i * 0.2) * 800,
      wau: 12400 + Math.sin(i * 0.15) * 1200,
      mau: 14200 + Math.sin(i * 0.1) * 1500,
    })),
    signups_per_week: [
      { week_start: "2026-09-01", signups: 940 },
      { week_start: "2026-08-25", signups: 1080 },
      { week_start: "2026-08-18", signups: 950 },
      { week_start: "2026-08-11", signups: 1200 },
    ],
    cumulative_users: baseDates.map((day, i) => ({
      day,
      users: 16890 + i * 51 + Math.floor(Math.sin(i * 0.2) * 200),
    })),
    funnel: {
      cohort_size: 1254,
      steps: [
        { key: "signup", label: "Inscription", users: 1254, eligible: 1254, excluded_too_recent: 0, conversion_pct: null },
        { key: "first_session", label: "Première séance", users: 892, eligible: 1254, excluded_too_recent: 0, conversion_pct: 71.1 },
        { key: "three_episodes", label: "3 épisodes distincts vus", users: 624, eligible: 892, excluded_too_recent: 0, conversion_pct: 69.96 },
        { key: "active_d7", label: "Actif à J7", users: 445, eligible: 580, excluded_too_recent: 100, conversion_pct: 76.72 },
        { key: "active_d30", label: "Actif à J30", users: 218, eligible: 412, excluded_too_recent: 200, conversion_pct: 52.91 },
      ],
      note: "nested funnel; Jn = session on signup day + n; accounts too recent for a step are excluded from its denominator (excluded_too_recent)",
    },
    cohorts: [
      { week_start: "2026-09-01", users: 340, d1: 68.5, d7: 45.2, d14: 32.1, d30: 18.8 },
      { week_start: "2026-08-25", users: 280, d1: 70.2, d7: 47.1, d14: 33.9, d30: 19.5 },
      { week_start: "2026-08-18", users: 310, d1: 69.8, d7: 46.5, d14: 32.8, d30: 19.2 },
      { week_start: "2026-08-11", users: 324, d1: 71.1, d7: 48.2, d14: 34.1, d30: 20.1 },
    ],
    time_to_first_session: {
      cohort_size: 1254,
      buckets: [
        { key: "lt_1h", label: "< 1 h", users: 380, share_pct: 30.3 },
        { key: "1h_24h", label: "1-24 h", users: 420, share_pct: 33.5 },
        { key: "1d_7d", label: "1-7 j", users: 280, share_pct: 22.3 },
        { key: "gt_7d", label: "> 7 j", users: 120, share_pct: 9.6 },
        { key: "never", label: "Jamais (aucune séance à ce jour)", users: 54, share_pct: 4.3 },
      ],
    },
    churn: {
      previous_window_active: 12800,
      churned: 1920,
      churn_pct: 15.0,
      window_days: 30,
    },
    not_measured: ["visitor_to_signup", "acquisition_sources"],
    visitor_to_signup: { measured: false, value: null },
    acquisition_sources: { measured: false, value: null },
  } as AdminGrowth,

  playbackHealth: {
    active_sessions: { value: 340, measured: true },
    error_rate: {
      value: 0.82,
      previous: 0.94,
      delta_pct: -12.77,
      errors: 102,
      sessions: 12450,
      measured: true,
      note: "erreurs enregistrées / séances ; seuls les sites instrumentés sont comptés",
    },
    startup_ms: { p50: null, p95: null, measured: false },
    sources: {
      active: null,
      total: null,
      failing: 4,
      measured: false,
    },
    cache: { data: null, measured: false },
  } as AdminPlaybackHealth,

  playbackErrors: {
    since: "2026-09-04T10:00:00Z",
    items: [
      {
        code: "stream_timeout",
        anime_id: 1,
        episode: 5,
        source: "nyaa",
        occurrences: 24,
        first_seen: "2026-09-01T08:30:00Z",
        last_seen: "2026-09-04T22:15:00Z",
      },
      {
        code: "remux_failed",
        anime_id: null,
        episode: null,
        source: null,
        occurrences: 18,
        first_seen: "2026-09-02T14:20:00Z",
        last_seen: "2026-09-04T19:45:00Z",
      },
      {
        code: "source_timeout",
        anime_id: 3,
        episode: 8,
        source: "sonarr",
        occurrences: 12,
        first_seen: "2026-09-03T10:00:00Z",
        last_seen: "2026-09-04T15:30:00Z",
      },
    ],
    page: { limit: 50, offset: 0, total: 54 },
  } as AdminPlaybackErrors,

  errorsSummary: {
    total: kpi(102, 118),
    by_code: [
      { code: "stream_timeout", count: kpi(24, 28) },
      { code: "remux_failed", count: kpi(18, 22) },
      { code: "source_timeout", count: kpi(12, 15) },
      { code: "kv_unavailable", count: kpi(8, 10) },
      { code: "source_dead", count: kpi(24, 28) },
      { code: "subtitle_failed", count: kpi(8, 10) },
      { code: "unknown", count: kpi(8, 5) },
    ],
    daily: baseDates.map((day, i) => {
      const total = 3 + Math.floor(Math.sin(i * 0.25) * 4);
      return {
        day,
        total,
        by_code: {
          stream_timeout: Math.ceil(total * 0.35),
          remux_failed: Math.ceil(total * 0.25),
          source_timeout: Math.ceil(total * 0.15),
          kv_unavailable: Math.ceil(total * 0.1),
          source_dead: Math.ceil(total * 0.1),
          subtitle_failed: Math.ceil(total * 0.05),
        },
      };
    }),
    probable_causes: [
      {
        code: "stream_timeout",
        count: 24,
        probable_cause: "Aucun pair ne répond ou le démarrage dépasse le délai : torrent peu seedé, pairs injoignables ou trackers morts.",
        files: ["internal/torrent", "internal/playback/manager.go"],
      },
      {
        code: "source_dead",
        count: 24,
        probable_cause: "Tous les fournisseurs de sources ont échoué : indexeur ou Sonarr/Radarr injoignable, clé API invalide ou limite de débit.",
        files: ["internal/indexer"],
      },
    ],
  } as AdminErrorsSummary,

  sources: {
    sources: [
      {
        source: "nyaa",
        failures: kpi(24, 28),
        share_pct: 23.5,
        last_seen: "2026-09-04T22:15:00Z",
        failure_rate: null,
        measured: false,
      },
      {
        source: "sonarr",
        failures: kpi(18, 22),
        share_pct: 17.6,
        last_seen: "2026-09-04T20:30:00Z",
        failure_rate: null,
        measured: false,
      },
      {
        source: "unknown",
        failures: kpi(12, 14),
        share_pct: 11.8,
        last_seen: "2026-09-04T18:45:00Z",
        failure_rate: null,
        measured: false,
      },
    ],
    total_failures: 102,
    not_measured: ["attempts_per_source"],
  } as AdminSources,

  costs: {
    usage: {
      watch_hours: kpi(47200, 39800),
      sessions: kpi(124300, 108420),
      active_users: kpi(9850, 8920),
      peak_concurrent_sessions: {
        value: 1240,
        estimated: true,
        truncated: false,
        method: "sweep over watch_sessions (started_at..updated_at)",
      },
    },
    costs: {
      currency_unit: "unit of the configured prices (no currency is assumed)",
      server: { value: 150.0, measured: true },
      bandwidth: { value: 425.5, measured: true },
      storage: { value: null, measured: false, missing_inputs: ["GAZES_COST_STORAGE_PER_GB_MONTH", "stored volume (not measured)"] },
      total: { value: 575.5, measured: true, partial: true },
      per_watch_hour: { value: 0.0122, measured: true },
      per_active_user: { value: 58.42, measured: true },
      server_prorata: 1.0,
    },
  } as AdminCosts,

  issuesList: {
    items: [
      {
        id: "iss-a7b2c9d1",
        created_at: "2026-09-02T14:30:00Z",
        updated_at: "2026-09-04T10:15:00Z",
        severity: "high" as const,
        status: "in_progress" as const,
        title: "Stream timeout on popular anime during peak hours",
        evidence: "24 occurrences of stream_timeout code on anime_id=1 (Frieren) between 20:00-23:00 UTC",
        suggested_fix: "Increase peer discovery timeout or add fallback torrent sources",
        source: "monitoring",
        note: "Customer impact: ~2-3% of peak hour sessions. Escalated to torrent agent.",
      },
      {
        id: "iss-b4e8f2a0",
        created_at: "2026-09-01T09:00:00Z",
        updated_at: "2026-09-04T08:30:00Z",
        severity: "medium" as const,
        status: "resolved" as const,
        title: "Subtitle extraction failures for specific video codecs",
        evidence: "8 occurrences of subtitle_failed on h265-encoded streams",
        suggested_fix: "Update ffmpeg to latest version or patch codec detection",
        source: "user-report",
        note: "Fixed in commit abc123. Monitor for regressions.",
      },
      {
        id: "iss-c3d5e1b6",
        created_at: "2026-08-30T16:45:00Z",
        updated_at: "2026-09-02T12:00:00Z",
        severity: "critical" as const,
        status: "new" as const,
        title: "Redis connection pool exhaustion under load",
        evidence: "kv_unavailable errors spike to 60+ per day during peak hours; connection logs show connection leaks",
        suggested_fix: "Review KV client pool configuration; implement connection timeout and cleanup",
        source: "internal",
      },
    ],
    page: { limit: 50, offset: 0, total: 3 },
  } as AdminIssuesList,

  issue: {
    id: "iss-a7b2c9d1",
    created_at: "2026-09-02T14:30:00Z",
    updated_at: "2026-09-04T10:15:00Z",
    severity: "high" as const,
    status: "in_progress" as const,
    title: "Stream timeout on popular anime during peak hours",
    evidence: "24 occurrences of stream_timeout code on anime_id=1 (Frieren) between 20:00-23:00 UTC",
    suggested_fix: "Increase peer discovery timeout or add fallback torrent sources",
    source: "monitoring",
    note: "Customer impact: ~2-3% of peak hour sessions. Escalated to torrent agent.",
  } as AdminIssue,
};
