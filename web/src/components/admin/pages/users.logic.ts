// Pure mapping from GET /users/summary, /users and /users/{id} to the props of the Utilisateurs blocks.
import type { AdminUserDetail, AdminUserRow, AdminUserSegment, AdminUsersList, AdminUsersSummary } from "@/lib/admin/types";
import { MONTHS_FR, fmtDec, fmtInt, relativeFr } from "./fr-date";

export const USERS_PAGE_SIZE = 25;

/** Sort keys accepted by GET /users. */
export const USER_SORT_KEYS = ["user_id", "created_at", "last_activity", "sessions", "watch_hours", "pseudo"] as const;
export type UserSortKey = (typeof USER_SORT_KEYS)[number];

export const DEFAULT_USER_SORT: UserSortKey = "last_activity";
export const MAX_QUERY_LENGTH = 64;

/** State of the directory, carried by the URL (?q=&sort=&dir=&page=&user=). */
export interface UsersQuery {
  q: string;
  sort: UserSortKey;
  dir: "asc" | "desc";
  /** 1-based page number. */
  page: number;
  /** Selected account (detail panel), or null. */
  user: number | null;
}

type RawParam = string | string[] | undefined | null;
export type RawSearchParams = { [key: string]: RawParam };

function first(raw: RawParam): string {
  return (Array.isArray(raw) ? raw[0] : raw) ?? "";
}

function positiveInt(raw: RawParam): number | null {
  const value = first(raw);
  if (!/^\d{1,12}$/.test(value)) return null;
  const n = Number(value);
  return Number.isSafeInteger(n) && n >= 1 ? n : null;
}

/** Reads the directory state from the raw search params; anything invalid falls back to the default. */
export function parseUsersQuery(params: RawSearchParams): UsersQuery {
  const sortRaw = first(params.sort);
  const sort = (USER_SORT_KEYS as readonly string[]).includes(sortRaw) ? (sortRaw as UserSortKey) : DEFAULT_USER_SORT;
  const dir = first(params.dir) === "asc" ? "asc" : "desc";
  return {
    q: first(params.q).trim().slice(0, MAX_QUERY_LENGTH),
    sort,
    dir,
    page: positiveInt(params.page) ?? 1,
    user: positiveInt(params.user),
  };
}

/** Query string of a state; defaults are left out so that the plain URL stays clean. */
export function usersQueryString(query: UsersQuery, extra: Record<string, string> = {}): string {
  const out = new URLSearchParams(extra);
  if (query.q) out.set("q", query.q);
  if (query.sort !== DEFAULT_USER_SORT) out.set("sort", query.sort);
  if (query.dir !== "desc") out.set("dir", query.dir);
  if (query.page > 1) out.set("page", String(query.page));
  if (query.user !== null) out.set("user", String(query.user));
  return out.toString();
}

/** Parameters of the GET /users call. */
export function usersApiParams(query: UsersQuery): Record<string, string | number> {
  const params: Record<string, string | number> = {
    sort: query.sort,
    dir: query.dir,
    limit: USERS_PAGE_SIZE,
    offset: (query.page - 1) * USERS_PAGE_SIZE,
  };
  if (query.q) params.q = query.q;
  return params;
}

export function pageCount(total: number, limit = USERS_PAGE_SIZE): number {
  return Math.max(1, Math.ceil(total / limit));
}

/** "26 à 50 sur 140 comptes". */
export function directoryStatus(list: Pick<AdminUsersList, "total" | "offset" | "users">, searching: boolean): string {
  if (list.total === 0) return searching ? "Aucun compte ne correspond" : "Aucun compte";
  const from = list.offset + 1;
  const to = list.offset + list.users.length;
  const noun = list.total > 1 ? "comptes" : "compte";
  return `${fmtInt(from)} à ${fmtInt(to)} sur ${fmtInt(list.total)} ${noun}`;
}

/** "2026-03-29T10:00:00Z" -> "29 mars 2026". */
export function dateFr(iso: string): string {
  const t = Date.parse(iso);
  if (!Number.isFinite(t)) return iso;
  const d = new Date(t);
  return `${d.getUTCDate()} ${MONTHS_FR[d.getUTCMonth()]} ${d.getUTCFullYear()}`;
}

export const SEGMENT_TONE: Record<AdminUserSegment, "accent" | "danger" | "neutral"> = {
  new: "accent",
  power: "accent",
  at_risk: "danger",
  regular: "neutral",
  dormant: "neutral",
  never_watched: "neutral",
};

export const SEGMENT_LABEL: Record<AdminUserSegment, string> = {
  new: "Nouveau",
  regular: "Régulier",
  power: "Power user",
  dormant: "Dormant",
  at_risk: "À risque",
  never_watched: "Jamais regardé",
};

export const SEGMENT_RULE: Record<string, string> = {
  new: "Compte créé il y a moins de 7 jours",
  regular: "Compte avec des séances qui ne relève d'aucun autre segment",
  power: "Plus de 10 h regardées par semaine en moyenne sur 30 jours",
  dormant: "Dernière séance il y a plus de 30 jours",
  at_risk: "Aucune séance depuis 14 à 30 jours, après au moins 4 séances le mois d'avant",
  never_watched: "Compte de plus de 7 jours sans aucune séance",
};

export function segmentLabel(segment: string): string {
  return (SEGMENT_LABEL as Record<string, string>)[segment] ?? segment;
}

export interface UsersKpi {
  label: string;
  value: number;
  note: string;
  delta: number | null;
  vs: string;
}

export function usersKpis(summary: AdminUsersSummary): UsersKpi[] {
  const k = summary.kpis;
  const total = k.total.value;
  const share = (v: number) => (total > 0 ? `${fmtDec((v / total) * 100, 1)} % des inscrits` : "");
  return [
    { label: "Inscrits", value: k.total.value, note: "Tous les comptes", delta: k.total.delta_pct, vs: "vs 30 j préc." },
    { label: "Actifs 7 j", value: k.active_7d.value, note: share(k.active_7d.value), delta: k.active_7d.delta_pct, vs: "vs 7 j préc." },
    { label: "Actifs 30 j", value: k.active_30d.value, note: share(k.active_30d.value), delta: k.active_30d.delta_pct, vs: "vs 30 j préc." },
    { label: "Dormants > 30 j", value: k.dormant.value, note: share(k.dormant.value), delta: k.dormant.delta_pct, vs: "vs 30 j préc." },
    { label: "Sans aucune séance", value: k.no_session.value, note: share(k.no_session.value), delta: k.no_session.delta_pct, vs: "vs 30 j préc." },
  ];
}

export interface UsersInsight {
  level: "haute" | "moyenne" | "basse";
  title: string;
  text: string;
}

function segmentOf(summary: AdminUsersSummary, key: string) {
  return summary.segments.find((s) => s.key === key);
}

/** Figures worth a look, drawn only from the summary (nothing is invented). */
export function usersInsights(summary: AdminUsersSummary): UsersInsight[] {
  const total = summary.kpis.total.value;
  if (total <= 0) return [];
  const out: UsersInsight[] = [];

  const never = summary.kpis.no_session.value;
  if (never > 0) {
    const pct = (never / total) * 100;
    out.push({
      level: pct >= 20 ? "haute" : pct >= 10 ? "moyenne" : "basse",
      title: `${fmtInt(never)} ${never > 1 ? "comptes" : "compte"} (${fmtDec(pct, 1)} %) ${never > 1 ? "n'ont" : "n'a"} jamais lancé de séance.`,
      text: "Vérifier le parcours juste après l'inscription : le premier écran mène-t-il à un épisode jouable ?",
    });
  }

  const risk = segmentOf(summary, "at_risk");
  if (risk && risk.users > 0) {
    out.push({
      level: "moyenne",
      title: `${fmtInt(risk.users)} ${risk.users > 1 ? "comptes à risque" : "compte à risque"} (${fmtDec(risk.share_pct, 1)} %), sans séance depuis 14 jours ou plus.`,
      text: "Les relancer avant qu'ils ne basculent en dormants, avec leur progression en cours comme point d'entrée.",
    });
  }

  const power = segmentOf(summary, "power");
  const hours = summary.segments.reduce((acc, s) => acc + s.users * s.avg_watch_hours, 0);
  if (power && power.users > 0 && hours > 0) {
    const powerShare = ((power.users * power.avg_watch_hours) / hours) * 100;
    out.push({
      level: "basse",
      title: `${fmtInt(power.users)} ${power.users > 1 ? "power users" : "power user"} (${fmtDec(power.share_pct, 1)} % des comptes) ${power.users > 1 ? "concentrent" : "concentre"} ${fmtInt(powerShare)} % du temps regardé.`,
      text: "Protéger leur expérience en priorité : surveiller les erreurs de lecture sur les animes qu'ils suivent.",
    });
  }

  const { valid, expiring_7d: soon } = summary.active_sessions;
  if (valid > 0 && soon / valid >= 0.3) {
    out.push({
      level: "moyenne",
      title: `${fmtInt(soon)} sessions sur ${fmtInt(valid)} expirent d'ici 7 jours.`,
      text: "Ces comptes devront se reconnecter : prévoir un message clair sur l'écran de connexion.",
    });
  }
  return out;
}

export interface SegmentRow {
  key: string;
  name: string;
  rule: string;
  users: number;
  share: number;
  hours: number;
  nameTone: "accent" | "danger" | "neutral";
}

export function segmentRows(summary: AdminUsersSummary): SegmentRow[] {
  return summary.segments.map((s) => ({
    key: s.key,
    name: s.label,
    rule: SEGMENT_RULE[s.key] ?? "",
    users: s.users,
    share: s.share_pct,
    hours: s.avg_watch_hours,
    nameTone: (SEGMENT_TONE as Record<string, "accent" | "danger" | "neutral">)[s.key] ?? "neutral",
  }));
}

/** Directory row: pseudo only when the API sent it, otherwise the account id. */
export function directoryRow(user: AdminUserRow, nowIso: string): Record<string, unknown> {
  return {
    rowId: user.user_id,
    pseudo: user.pseudo ?? `#${user.user_id}`,
    user_id: `#${user.user_id}`,
    created_at: dateFr(user.created_at),
    last_activity: user.last_activity ? relativeFr(user.last_activity, nowIso) || dateFr(user.last_activity) : "Jamais",
    sessions: user.sessions,
    watch_hours: user.watch_hours,
    top: user.top_anime ? user.top_anime.title : "",
    segment: segmentLabel(user.segment),
    segmentTone: SEGMENT_TONE[user.segment],
  };
}

export interface DetailTile {
  label: string;
  value: string;
  mono?: boolean;
}

/** Tiles of the detail panel; figures that the detail endpoint lacks come from the directory row when the account is on the page. */
export function detailTiles(detail: AdminUserDetail, row: AdminUserRow | undefined, nowIso: string): DetailTile[] {
  const last = detail.recent_sessions[0]?.started_at ?? row?.last_activity ?? null;
  const tiles: DetailTile[] = [{ label: "Identifiant", value: `#${detail.user_id}`, mono: true }];
  if (detail.pseudo) tiles.push({ label: "Pseudo", value: detail.pseudo });
  tiles.push({ label: "Inscrit le", value: dateFr(detail.created_at) });
  tiles.push({ label: "Dernière activité", value: last ? relativeFr(last, nowIso) || dateFr(last) : "Jamais" });
  if (row) {
    tiles.push({ label: "Séances", value: fmtInt(row.sessions) });
    tiles.push({ label: "Heures regardées", value: `${fmtDec(row.watch_hours, 1)} h` });
    tiles.push({ label: "Anime le plus vu", value: row.top_anime ? row.top_anime.title : "Aucun" });
  }
  tiles.push({ label: "Segment", value: segmentLabel(detail.segment) });
  return tiles;
}

export function minutesText(seconds: number): string {
  const m = Math.round(seconds / 60);
  return m < 1 ? "< 1 min" : `${fmtInt(m)} min`;
}

/** 123.5 s -> "2 min 03 s". */
export function positionText(seconds: number): string {
  const total = Math.max(0, Math.round(seconds));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m} min ${String(s).padStart(2, "0")} s`;
}

export interface HistoryItem {
  key: string;
  title: string;
  episode: string;
  when: string;
  minutes: string;
  status: string;
}

export function historyItems(detail: AdminUserDetail, nowIso: string): HistoryItem[] {
  return detail.recent_sessions.map((s, i) => ({
    key: `${s.anime_id}-${s.episode}-${s.started_at}-${i}`,
    title: s.title,
    episode: `Épisode ${s.episode}`,
    when: relativeFr(s.started_at, nowIso) || dateFr(s.started_at),
    minutes: minutesText(s.watched_seconds),
    status: s.completed ? "Terminé" : "Interrompu",
  }));
}

export interface ProgressItem {
  key: string;
  label: string;
  text: string;
}

export function progressItems(detail: AdminUserDetail): ProgressItem[] {
  return detail.progress.map((p) => ({
    key: `${p.season_id}-${p.episode}`,
    label: `${p.title} · Épisode ${p.episode}`,
    text: `Reprise à ${positionText(p.position)} · mis à jour le ${dateFr(p.updated_at)}`,
  }));
}

export function seniorityItems(summary: AdminUsersSummary): Array<{ label: string; value: number; hint: string }> {
  return summary.seniority.map((s) => ({ label: s.label, value: s.users, hint: `${fmtDec(s.share_pct, 1)} %` }));
}
