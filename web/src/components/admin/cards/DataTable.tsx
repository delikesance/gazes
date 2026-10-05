"use client";

import React, { useEffect, useId, useMemo, useRef, useState } from "react";
import { Chip } from "../ui/Chip";
import type { ChipTone } from "../ui/Chip";
import { SeverityBadge } from "../ui/SeverityBadge";
import type { SeverityLevel } from "../ui/SeverityBadge";
import { Thumb } from "../ui/Thumb";
import {
  ALL_TAB,
  ariaSortOf,
  barMax,
  barPercent,
  columnAlign,
  countText,
  filterBySearch,
  filterByTab,
  formatDeltaCell,
  formatNumberCell,
  nextSort,
  plainValue,
  rowKeyOf,
  searchFieldId,
  sortRows,
} from "./cards.logic";
import type { CellType, ColumnDef, Row, SortDir, TabDef, Tone } from "./cards.logic";

export type { CellType, ColumnDef, Row, SortDir, TabDef } from "./cards.logic";

export interface DataTableProps {
  title?: string;
  subtitle?: string;
  columns: ColumnDef[];
  /**
   * Row values per column key. Optional companions: `<key>Sort` (number or ISO date used for sorting),
   * `<key>Tone` (accent|danger|muted), `<key>Kind` (status: chip|severity), `<key>Disabled` (toggle).
   */
  rows: Row[];
  rowKey?: string;
  sortKey?: string;
  sortDir?: SortDir;
  searchable?: boolean;
  searchLabel?: string;
  searchPlaceholder?: string;
  searchKeys?: string[];
  tabs?: TabDef[];
  tabKey?: string;
  tabAll?: boolean;
  tabAllLabel?: string;
  tabsLabel?: string;
  selectable?: boolean;
  selectedKey?: string;
  onSelect?: (row: Row) => void;
  minWidth?: number;
  maxHeight?: number;
  caption?: string;
  emptyText?: string;
  footnote?: string;
  grow?: number;
  basis?: number;
  /** Stable id fragment for the search field; a unique one is generated otherwise. */
  id?: string;
  onCellClick?: (row: Row, key: string) => void;
  onCellChange?: (row: Row, key: string, value: boolean) => void;
  /**
   * Server-driven mode: `rows` arrive already searched, sorted and paged. The table never filters or sorts
   * them itself; `sortKey`/`sortDir`/`searchValue` mirror the server state and the user's intent is reported
   * through `onSortChange` / `onSearch` (debounced while typing, immediate on Enter).
   */
  manual?: boolean;
  onSortChange?: (key: string, dir: SortDir) => void;
  searchValue?: string;
  onSearch?: (query: string) => void;
  /** Replaces the "N lignes" counter (e.g. "26 à 50 sur 140 comptes"). */
  statusText?: string;
  /** Rendered under the table (pagination controls, ...). */
  footer?: React.ReactNode;
  /** A server round trip is in flight: the table is marked aria-busy and dimmed. */
  busy?: boolean;
}

const SEARCH_DEBOUNCE_MS = 350;

const TONE_COLOR: Record<Tone, string> = { accent: "#9b8afb", danger: "#f87171", muted: "#a1a1aa" };
const BAR_COLOR: Record<Tone, string> = { accent: "#9b8afb", danger: "#f87171", muted: "#71717a" };
const SHAPE: Record<Tone, string> = { accent: "M5 0L10 5L5 10L0 5Z", danger: "M5 0L10 10H0Z", muted: "M1 1H9V9H1Z" };
const SEVERITIES: SeverityLevel[] = ["haute", "moyenne", "basse", "info"];

const isTone = (t: unknown): t is Tone => t === "accent" || t === "danger" || t === "muted";
const FOCUS = "focus-visible:outline-2 focus-visible:outline-offset-[3px] focus-visible:outline-[#fafafa] disabled:cursor-not-allowed disabled:opacity-50";
const MONO = "'Geist Mono', monospace";

const toChipTone = (t: string): ChipTone => (t === "accent" || t === "danger" || t === "code" ? t : "neutral");
const obj = (v: unknown): Record<string, unknown> | null => (v && typeof v === "object" ? (v as Record<string, unknown>) : null);

interface CellCtx {
  col: ColumnDef;
  row: Row;
  rowId: string;
  first: boolean;
  firstText: string;
  max: number;
  toggleOn: boolean;
  onToggle: () => void;
  onCellClick?: (row: Row, key: string) => void;
}

function CellContent({ col, row, first, firstText, max, toggleOn, onToggle, onCellClick }: CellCtx): React.ReactElement {
  const type: CellType = col.type || "text";
  const v = row[col.key];
  const empty = v == null || v === "";
  const rt = row[col.key + "Tone"];
  const rowTone = isTone(rt) ? rt : null;
  const colTone = isTone(col.tone) ? col.tone : null;
  const label = col.label == null ? col.key : String(col.label);

  const spanBase: React.CSSProperties = { display: "inline-flex", alignItems: "center", gap: 4 };

  if (empty) return <span style={{ ...spanBase, color: "#a1a1aa" }}>—</span>;

  if (type === "bar") {
    const n = typeof v === "object" ? NaN : Number(v);
    const ok = Number.isFinite(n);
    const text = ok ? formatNumberCell(n, col) : String(plainValue(v));
    const pct = ok ? barPercent(n, max) : 0;
    const tone: Tone = rowTone || colTone || "accent";
    return (
      <span style={{ display: "inline-flex", alignItems: "center", gap: 10, width: "100%" }}>
        <span style={{ display: "block", flex: "1 1 80px", minWidth: 56, maxWidth: 160, height: 6, borderRadius: 999, background: "#1c1c1f" }}>
          <span style={{ display: "block", height: 6, borderRadius: 999, minWidth: ok && n > 0 ? 6 : 0, width: pct.toFixed(1) + "%", background: BAR_COLOR[tone] }} />
        </span>
        <span
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: 4,
            flex: "none",
            fontVariantNumeric: "tabular-nums",
            ...(rowTone ? { fontWeight: 600, color: TONE_COLOR[rowTone] } : null),
          }}
        >
          {rowTone && (
            <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" style={{ flex: "none" }}>
              <path d={SHAPE[rowTone]} fill="currentColor"></path>
            </svg>
          )}
          {text}
        </span>
      </span>
    );
  }

  if (type === "thumb") {
    const text = String(plainValue(v));
    return (
      <span style={{ display: "inline-flex", alignItems: "center", gap: 10 }}>
        <Thumb title={text} decorative />
        <span
          style={{
            ...spanBase,
            ...(first ? { fontWeight: 500 } : null),
            ...(rowTone ? { color: TONE_COLOR[rowTone] } : colTone ? { color: TONE_COLOR[colTone] } : null),
          }}
        >
          {text}
        </span>
      </span>
    );
  }

  if (type === "button") {
    const lab = String(plainValue(v));
    const o = obj(v);
    const ariaLabel = o && o.ariaLabel ? String(o.ariaLabel) : firstText ? lab + " : " + firstText : lab;
    return (
      <button
        type="button"
        className={FOCUS}
        aria-label={ariaLabel}
        disabled={!!(o && o.disabled)}
        onClick={() => onCellClick?.(row, col.key)}
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          boxSizing: "border-box",
          minHeight: 36,
          padding: "0 16px",
          border: 0,
          borderRadius: 999,
          fontFamily: "inherit",
          fontSize: 13,
          fontWeight: 500,
          whiteSpace: "nowrap",
          cursor: "pointer",
          ...(rowTone === "danger"
            ? { background: "#7f1d1d", color: "#fecaca" }
            : { background: "#151517", boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)", color: rowTone ? TONE_COLOR[rowTone] : "#fafafa" }),
        }}
      >
        {lab}
      </button>
    );
  }

  if (type === "toggle") {
    const danger = rowTone === "danger";
    const accent = danger ? "#f87171" : "#9b8afb";
    const dis = row[col.key + "Disabled"];
    const disabled = dis === true || dis === "true";
    return (
      <button
        type="button"
        role="switch"
        className={FOCUS}
        aria-checked={toggleOn}
        aria-label={label + (firstText ? " : " + firstText : "")}
        disabled={disabled}
        onClick={onToggle}
        style={{
          display: "inline-flex",
          alignItems: "center",
          justifyContent: "center",
          boxSizing: "border-box",
          minWidth: 44,
          minHeight: 44,
          padding: 0,
          border: 0,
          borderRadius: 999,
          background: "transparent",
          cursor: "pointer",
        }}
      >
        <span
          style={{
            position: "relative",
            display: "block",
            flex: "none",
            width: 44,
            height: 24,
            borderRadius: 999,
            boxSizing: "border-box",
            ...(toggleOn ? { background: accent } : { background: "#17171a", boxShadow: "inset 0 0 0 1px #71717a" }),
          }}
        >
          <span
            style={{
              position: "absolute",
              top: 3,
              left: toggleOn ? 23 : 3,
              width: 18,
              height: 18,
              borderRadius: 999,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              background: toggleOn ? "#111111" : "#a1a1aa",
            }}
          >
            {toggleOn && (
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke={accent} strokeWidth="3.2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M20 6L9 17l-5-5"></path>
              </svg>
            )}
          </span>
        </span>
        <span style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" }}>
          {toggleOn ? "Activé" : "Désactivé"}
        </span>
      </button>
    );
  }

  if (type === "status") {
    const text = String(plainValue(v));
    const kindRaw = row[col.key + "Kind"] || col.kind || "chip";
    if (kindRaw === "severity") {
      const lvl = text.toLowerCase() as SeverityLevel;
      return <SeverityBadge level={SEVERITIES.includes(lvl) ? lvl : "info"} />;
    }
    const tone = rowTone ? (rowTone === "muted" ? "neutral" : rowTone) : (col.tones && col.tones[text]) || col.tone || "neutral";
    return <Chip label={text} tone={toChipTone(tone)} />;
  }

  if (type === "chip") {
    const text = String(v);
    const tone = rowTone ? (rowTone === "muted" ? "neutral" : rowTone) : (col.tones && col.tones[text]) || col.tone || "neutral";
    return <Chip label={text} tone={toChipTone(tone)} />;
  }

  // Plain-ish types: text, mono, number, delta
  let text = "";
  let style: React.CSSProperties = { ...spanBase };
  let arrow: string | null = null;
  let toned = false;

  if (type === "number") {
    text = formatNumberCell(v, col);
    style = { ...style, fontVariantNumeric: "tabular-nums" };
    toned = true;
  } else if (type === "delta") {
    const d = formatDeltaCell(v, col);
    text = d.text;
    style = {
      ...style,
      fontWeight: 600,
      fontVariantNumeric: "tabular-nums",
      color: d.tone === "flat" ? "#a1a1aa" : d.tone === "good" ? "#9b8afb" : "#f87171",
    };
    if (d.direction !== "flat") arrow = d.direction === "up" ? "M12 19V5 M5 12l7-7 7 7" : "M12 5v14 M19 12l-7 7-7-7";
  } else if (type === "mono") {
    text = String(v);
    style = { ...style, fontFamily: MONO, fontSize: 12, color: "#a1a1aa", whiteSpace: "nowrap" };
    toned = true;
  } else {
    text = String(v);
    if (first) style = { ...style, fontWeight: 500 };
    else if (col.muted) style = { ...style, color: "#a1a1aa" };
    toned = true;
  }
  if (col.mono && type === "text") style = { ...style, fontFamily: MONO, fontSize: 12, color: "#a1a1aa" };

  let shape: string | null = null;
  if (rowTone) {
    style = { ...style, color: TONE_COLOR[rowTone] };
    if (!arrow) shape = SHAPE[rowTone];
  } else if (colTone && toned) {
    style = { ...style, color: TONE_COLOR[colTone] };
  }

  return (
    <span style={style}>
      {shape && (
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" style={{ flex: "none" }}>
          <path d={shape} fill="currentColor"></path>
        </svg>
      )}
      {arrow && (
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" style={{ flex: "none" }}>
          <path d={arrow}></path>
        </svg>
      )}
      {text}
    </span>
  );
}

export const DataTable: React.FC<DataTableProps> = ({
  title = "",
  subtitle = "",
  columns,
  rows,
  rowKey = "id",
  sortKey: sortKeyProp = "",
  sortDir: sortDirProp = "asc",
  searchable = false,
  searchLabel = "Rechercher",
  searchPlaceholder = "",
  searchKeys,
  tabs = [],
  tabKey = "",
  tabAll = true,
  tabAllLabel = "Tous",
  tabsLabel = "Filtrer les lignes",
  selectable = false,
  selectedKey = "",
  onSelect,
  minWidth = 560,
  maxHeight = 0,
  caption,
  emptyText = "Aucune ligne ne correspond.",
  footnote = "",
  grow = 1,
  basis = 460,
  id,
  onCellClick,
  onCellChange,
  manual = false,
  onSortChange,
  searchValue = "",
  onSearch,
  statusText,
  footer,
  busy = false,
}) => {
  const uid = useId();
  const [sortState, setSortState] = useState<{ key: string; dir: SortDir }>({
    key: sortKeyProp,
    dir: sortDirProp === "desc" ? "desc" : "asc",
  });
  const sort = manual ? { key: sortKeyProp, dir: (sortDirProp === "desc" ? "desc" : "asc") as SortDir } : sortState;
  const [query, setQuery] = useState(manual ? searchValue : "");
  const lastSent = useRef(manual ? searchValue : "");
  const debounce = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => { if (debounce.current) clearTimeout(debounce.current); }, []);
  // The URL (back/forward) changed the query behind our back: follow it.
  useEffect(() => {
    if (manual && searchValue !== lastSent.current) {
      lastSent.current = searchValue;
      setQuery(searchValue);
    }
  }, [manual, searchValue]);
  const sendSearch = (value: string) => {
    if (debounce.current) clearTimeout(debounce.current);
    debounce.current = null;
    if (value.trim() === lastSent.current.trim()) return;
    lastSent.current = value;
    onSearch?.(value);
  };
  const [tab, setTab] = useState<string>(ALL_TAB);
  const [sel, setSel] = useState<string>(selectedKey == null ? "" : String(selectedKey));
  const [toggles, setToggles] = useState<Record<string, { base: boolean; val: boolean }>>({});

  const cols = useMemo<ColumnDef[]>(() => columns.map((c) => ({ type: "text" as CellType, ...c })), [columns]);
  const tabOn = tabs.length > 0 && tabKey !== "";
  const showToolbar = searchable || tabOn;

  const visible = useMemo(() => {
    if (manual) return rows;
    let list = rows;
    if (tabOn) list = filterByTab(list, tabKey, tab);
    if (searchable) {
      const keys = searchKeys && searchKeys.length ? searchKeys : cols.map((c) => c.key);
      list = filterBySearch(list, query, keys);
    }
    if (cols.some((c) => c.key === sort.key)) list = sortRows(list, sort.key, sort.dir);
    return list;
  }, [manual, rows, tabOn, tabKey, tab, searchable, searchKeys, cols, query, sort]);

  // Original row index for the rowKey fallback, independent of filtering/sorting.
  const indexOf = useMemo(() => {
    const m = new Map<Row, number>();
    rows.forEach((r, i) => m.set(r, i));
    return m;
  }, [rows]);

  const maxByCol = useMemo(() => {
    const out: Record<string, number> = {};
    for (const c of cols) if (c.type === "bar") out[c.key] = barMax(rows, c);
    return out;
  }, [cols, rows]);

  const firstText = (row: Row): string => {
    const fc = cols[0];
    if (!fc) return "";
    const t = plainValue(row[fc.key]);
    return t == null ? "" : String(t);
  };

  const tabList: TabDef[] = tabOn
    ? [...(tabAll ? [{ label: tabAllLabel, value: ALL_TAB }] : []), ...tabs]
    : [];

  const searchId = searchFieldId(uid, id);
  const heading = title || subtitle;
  const total = rows.length;
  const shown = visible.length;

  const renderCells = (row: Row, rowId: string, ci: number) => {
    const c = cols[ci];
    const raw = row[c.key];
    const rawOn = obj(raw) ? !!obj(raw)?.checked : raw === true || raw === "true";
    const tkey = rowId + "|" + c.key;
    const ov = toggles[tkey];
    const on = ov && ov.base === rawOn ? ov.val : rawOn;
    return (
      <CellContent
        col={c}
        row={row}
        rowId={rowId}
        first={ci === 0}
        firstText={firstText(row)}
        max={maxByCol[c.key] ?? 1}
        toggleOn={on}
        onToggle={() => {
          const nv = !on;
          setToggles((t) => ({ ...t, [tkey]: { base: rawOn, val: nv } }));
          onCellChange?.(row, c.key, nv);
        }}
        onCellClick={onCellClick}
      />
    );
  };

  const cellPad = (c: ColumnDef, ci: number): React.CSSProperties => ({
    padding: ci === 0 ? "4px 12px 4px 0" : ci === cols.length - 1 ? "12px 0 12px 12px" : "12px",
    textAlign: columnAlign(c),
    ...(c.width ? { width: c.width } : null),
    ...(ci === 0 ? { fontWeight: 500 } : null),
  });

  return (
    <section
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 16,
        flex: `${grow} 1 ${basis}px`,
        width: "100%",
        minWidth: 0,
        boxSizing: "border-box",
        padding: 24,
        borderRadius: 20,
        background: "#111113",
        boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)",
        fontFamily: "'DM Sans', system-ui, sans-serif",
        fontSize: 14,
        lineHeight: 1.4,
        color: "#fafafa",
      }}
    >
      {heading && (
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", justifyContent: "space-between", gap: 8 }}>
          <h2 style={{ margin: 0, fontSize: 24, fontWeight: 500, letterSpacing: "-0.02em", lineHeight: "30px" }}>{title}</h2>
          <span style={{ fontSize: 13, color: "#a1a1aa" }}>{subtitle}</span>
        </div>
      )}
      {showToolbar && (
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "flex-end", justifyContent: "space-between", gap: 12 }}>
          {searchable && (
            <div style={{ flex: "1 1 280px", maxWidth: 420, display: "flex", flexDirection: "column", gap: 6 }}>
              <label
                htmlFor={searchId}
                style={{ fontFamily: MONO, fontSize: 11, fontWeight: 500, letterSpacing: "0.08em", textTransform: "uppercase", color: "#a1a1aa" }}
              >
                {searchLabel}
              </label>
              <input
                id={searchId}
                type="search"
                autoComplete="off"
                placeholder={searchPlaceholder}
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  if (manual) {
                    if (debounce.current) clearTimeout(debounce.current);
                    const next = e.target.value;
                    debounce.current = setTimeout(() => sendSearch(next), SEARCH_DEBOUNCE_MS);
                  }
                }}
                onKeyDown={(e) => {
                  if (manual && e.key === "Enter") sendSearch(e.currentTarget.value);
                }}
                className="focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#fafafa]"
                style={{
                  height: 44,
                  boxSizing: "border-box",
                  width: "100%",
                  padding: "0 16px",
                  border: 0,
                  borderRadius: 999,
                  background: "#17171a",
                  boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)",
                  color: "#fafafa",
                  fontFamily: "inherit",
                  fontSize: 14,
                }}
              />
            </div>
          )}
          {tabOn && (
            <div role="group" aria-label={tabsLabel} style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
              {tabList.map((t) => {
                const on = String(tab) === String(t.value);
                return (
                  <button
                    key={t.value}
                    type="button"
                    aria-pressed={on}
                    onClick={() => setTab(t.value)}
                    className={FOCUS}
                    style={{
                      minHeight: 36,
                      padding: "0 16px",
                      border: 0,
                      borderRadius: 999,
                      fontFamily: "inherit",
                      fontSize: 13,
                      fontWeight: 500,
                      cursor: "pointer",
                      ...(on
                        ? { background: "#fafafa", color: "#111111" }
                        : { background: "#151517", color: "#a1a1aa", boxShadow: "inset 0 0 0 1px rgba(255,255,255,0.07)" }),
                    }}
                  >
                    {t.label}
                  </button>
                );
              })}
            </div>
          )}
          <span role="status" style={{ fontSize: 12, color: "#a1a1aa" }}>
            {statusText ?? countText(shown, total)}
          </span>
        </div>
      )}
      <div
        aria-busy={busy || undefined}
        style={{ ...(maxHeight > 0 ? { overflow: "auto", maxHeight, borderRadius: 12 } : { overflowX: "auto" }), ...(busy ? { opacity: 0.6 } : null) }}
      >
        <table style={{ width: "100%", minWidth, borderCollapse: "collapse", fontSize: 13 }}>
          <caption style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" }}>
            {caption || title || "Tableau de données"}
          </caption>
          <thead>
            <tr style={{ textAlign: "left", fontFamily: MONO, fontSize: 11, letterSpacing: "0.08em", textTransform: "uppercase" }}>
              {cols.map((c, ci) => {
                const active = sort.key === c.key;
                const sortable = c.sortable !== false;
                const al = columnAlign(c);
                const lab = c.label == null ? c.key : String(c.label);
                const arrow = active
                  ? sort.dir === "asc"
                    ? "M12 19V5 M5 12l7-7 7 7"
                    : "M12 5v14 M19 12l-7 7-7-7"
                  : "M7 10l5-5 5 5 M7 14l5 5 5-5";
                return (
                  <th
                    key={c.key}
                    scope="col"
                    aria-sort={ariaSortOf(active, sort.dir)}
                    style={{
                      position: "sticky",
                      top: 0,
                      zIndex: 1,
                      background: "#111113",
                      padding: `4px ${ci === cols.length - 1 ? 0 : 12}px 4px ${ci === 0 ? 0 : 12}px`,
                      fontWeight: 500,
                      color: active ? "#fafafa" : "#a1a1aa",
                      textAlign: al,
                      ...(c.width ? { width: c.width } : null),
                    }}
                  >
                    {sortable ? (
                      <button
                        type="button"
                        onClick={() => {
                          const next = nextSort(sort, c);
                          if (manual) onSortChange?.(next.key, next.dir);
                          else setSortState(next);
                        }}
                        className={FOCUS}
                        style={{
                          display: "inline-flex",
                          alignItems: "center",
                          gap: 6,
                          minHeight: 44,
                          padding: 0,
                          border: 0,
                          background: "none",
                          color: "inherit",
                          font: "inherit",
                          letterSpacing: "inherit",
                          textTransform: "inherit",
                          cursor: "pointer",
                          flexDirection: al === "right" ? "row-reverse" : "row",
                        }}
                      >
                        {lab}
                        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" style={{ flex: "none", opacity: active ? 1 : 0.5 }}>
                          <path d={arrow}></path>
                        </svg>
                      </button>
                    ) : (
                      <span style={{ display: "inline-block", padding: "12px 0" }}>{lab}</span>
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {visible.map((row) => {
              const rowId = rowKeyOf(row, indexOf.get(row) ?? 0, rowKey);
              const isSel = selectable && sel === rowId;
              return (
                <tr
                  key={rowId}
                  style={{ borderTop: "1px solid rgba(255,255,255,0.07)", background: isSel ? "#17171a" : "transparent" }}
                >
                  {cols.map((c, ci) => {
                    const content = renderCells(row, rowId, ci);
                    if (ci === 0) {
                      return (
                        <th key={c.key} scope="row" style={{ ...cellPad(c, ci), textAlign: columnAlign(c) }}>
                          {selectable ? (
                            <button
                              type="button"
                              aria-pressed={isSel}
                              className={FOCUS}
                              onClick={() => {
                                setSel(rowId);
                                onSelect?.(row);
                              }}
                              style={{
                                minHeight: 44,
                                padding: 0,
                                border: 0,
                                background: "none",
                                color: "#fafafa",
                                fontFamily: "inherit",
                                fontSize: 13,
                                fontWeight: 500,
                                cursor: "pointer",
                                textAlign: "left",
                                ...(isSel ? { textDecoration: "underline", textUnderlineOffset: 4 } : null),
                              }}
                            >
                              {content}
                            </button>
                          ) : (
                            content
                          )}
                        </th>
                      );
                    }
                    return (
                      <td key={c.key} style={cellPad(c, ci)}>
                        {content}
                      </td>
                    );
                  })}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      {shown === 0 && (
        <p role="status" style={{ margin: 0, padding: "8px 0", fontSize: 13, color: "#a1a1aa" }}>
          {emptyText}
        </p>
      )}
      {footer}
      {footnote && <span style={{ fontSize: 12, color: "#a1a1aa" }}>{footnote}</span>}
    </section>
  );
};

DataTable.displayName = "DataTable";
