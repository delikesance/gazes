"use client";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { Check, ChevronLeft, ChevronRight, CalendarX } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { getSchedule } from "@/lib/api";
import { addDays, airingStates, apiBounds, dayKey, daysIn, groupByDay, isSameDay, monthGridRange, startOfDay, weekRange, type AiringState } from "@/lib/calendar";
import type { ScheduleEntry } from "@/types/api";
import { LazyImage } from "./ui/LazyImage";

type View = "week" | "month" | "list";

interface Loaded { key: string; entries: ScheduleEntry[]; partial: boolean; error: boolean }

const VIEWS: [View, string][] = [["week", "Semaine"], ["month", "Mois"], ["list", "Liste"]];

function entryHref(entry: ScheduleEntry, state: AiringState): string {
  return state === "aired" ? `/anime/${entry.media_id}/seasons/${entry.media_id}/episodes/${entry.episode}` : `/anime/${entry.media_id}`;
}

/** Release calendar: week columns, month grid with a day panel, or a plain list. */
export function ReleaseCalendar() {
  const { t, locale } = useI18n();
  const [view, setView] = useState<View>("week");
  const [anchor, setAnchor] = useState(() => startOfDay(new Date()));
  const [selected, setSelected] = useState(() => startOfDay(new Date()));
  const [loaded, setLoaded] = useState<Loaded | null>(null);
  const [retry, setRetry] = useState(0);

  const today = startOfDay(new Date());
  const range = useMemo(() => (view === "month" ? monthGridRange(anchor) : weekRange(anchor)), [view, anchor]);
  const key = `${range.start.getTime()}-${range.end.getTime()}-${retry}`;
  const days = useMemo(() => daysIn(range), [range]);

  useEffect(() => {
    const controller = new AbortController();
    const { from, to } = apiBounds(range);
    getSchedule(from, to, controller.signal).then(
      (result) => setLoaded({ key, entries: result.entries || [], partial: Boolean(result.partial), error: false }),
      (error) => { if (error?.name !== "AbortError") setLoaded({ key, entries: [], partial: false, error: true }); },
    );
    return () => controller.abort();
  }, [key, range]); // eslint-disable-line react-hooks/exhaustive-deps

  const loading = loaded?.key !== key;
  const entries = useMemo(() => (loaded?.key === key ? loaded.entries : []), [loaded, key]);
  const byDay = useMemo(() => groupByDay(entries), [entries]);
  const states = useMemo(() => airingStates(entries, new Date()), [entries]);

  const time = useMemo(() => new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit" }), [locale]);
  const weekday = useMemo(() => new Intl.DateTimeFormat(locale, { weekday: "short" }), [locale]);
  const weekdayLong = useMemo(() => new Intl.DateTimeFormat(locale, { weekday: "long" }), [locale]);
  const dayMonth = useMemo(() => new Intl.DateTimeFormat(locale, { day: "numeric", month: "long" }), [locale]);
  const shortDate = useMemo(() => new Intl.DateTimeFormat(locale, { day: "numeric", month: "short" }), [locale]);
  const monthYear = useMemo(() => new Intl.DateTimeFormat(locale, { month: "long", year: "numeric" }), [locale]);

  const thisWeek = weekRange(today).start.getTime() === weekRange(anchor).start.getTime();
  const title = view === "month" ? monthYear.format(anchor) : thisWeek ? t("Cette semaine") : `${t("Semaine du")} ${shortDate.format(range.start)}`;
  const subtitle = view === "month" ? "" : `${shortDate.format(range.start)} – ${shortDate.format(addDays(range.end, -1))}`;

  const step = (direction: number) => {
    const next = view === "month" ? new Date(anchor.getFullYear(), anchor.getMonth() + direction, 1) : addDays(anchor, 7 * direction);
    setAnchor(next);
    setSelected(view === "month" ? next : weekRange(next).start);
  };
  const goToday = () => { setAnchor(today); setSelected(today); };

  const entriesOf = (day: Date) => byDay.get(dayKey(day)) || [];

  const renderCard = (entry: ScheduleEntry) => {
    const state = states.get(entry) || "upcoming";
    const hour = new Date(entry.airing_at * 1000).getHours();
    return (
      <Link key={`${entry.media_id}-${entry.episode}`} href={entryHref(entry, state)} className="cal-card" data-state={state}>
        <span className="cal-thumb">
          <LazyImage src={entry.poster_image} alt="" aspectRatio="" className="cal-thumb-art" />
          <span className="chip cal-chip cal-chip-ep">{t("Épisode")} {entry.episode}</span>
          {state === "aired" && <span className="chip cal-chip cal-chip-state"><Check size={11} aria-hidden="true" />{t("Diffusé")}</span>}
          {state === "next" && <span className="chip cal-chip cal-chip-state cal-chip-accent">{t(hour >= 17 ? "Ce soir" : "Aujourd’hui")}</span>}
          <span className="chip cal-chip cal-chip-time">{time.format(new Date(entry.airing_at * 1000))}</span>
        </span>
        <span className="cal-title">{entry.title}</span>
        {(entry.studio || entry.genres?.[0]) && <span className="cal-meta">{[entry.studio, entry.genres?.[0]].filter(Boolean).join(" · ")}</span>}
      </Link>
    );
  }

  const renderRow = (entry: ScheduleEntry) => {
    const state = states.get(entry) || "upcoming";
    const hour = new Date(entry.airing_at * 1000).getHours();
    return (
      <Link key={`${entry.media_id}-${entry.episode}`} href={entryHref(entry, state)} className="cal-row" data-state={state}>
        <span className="cal-row-poster"><LazyImage src={entry.poster_image} alt="" aspectRatio="" className="cal-thumb-art" /></span>
        <span className="cal-row-copy">
          <span className="cal-title">{entry.title}</span>
          <span className="cal-row-chips">
            <span className="chip">{t("Épisode")} {entry.episode}</span>
            <span className="chip">{time.format(new Date(entry.airing_at * 1000))}</span>
            {state === "aired" && <span className="chip"><Check size={11} aria-hidden="true" />{t("Diffusé")}</span>}
            {state === "next" && <span className="chip cal-chip-accent">{t(hour >= 17 ? "Ce soir" : "Aujourd’hui")}</span>}
          </span>
        </span>
      </Link>
    );
  };

  const empty = (
    <div className="cal-empty"><CalendarX size={20} aria-hidden="true" /><span>{t("Aucune sortie ce jour-là.")}</span></div>
  );

  const selectedEntries = entriesOf(selected);
  const dayPanel = (
    <div className="cal-day-panel">
      <div className="cal-day-head">
        <span className="eyebrow">{weekdayLong.format(selected)}</span>
        <h3 className="serif">{dayMonth.format(selected)}</h3>
        <span className="cal-count">{t(selectedEntries.length === 1 ? "{count} sortie" : "{count} sorties", { count: selectedEntries.length })}</span>
      </div>
      <div className="cal-rows">{selectedEntries.length ? selectedEntries.map(renderRow) : empty}</div>
    </div>
  );

  return (
    <section className="release-calendar page-inset" aria-labelledby="calendar-heading" aria-busy={loading}>
      <div className="cal-header">
        <div className="cal-heading">
          <span className="eyebrow">{t("Calendrier")}</span>
          <h2 id="calendar-heading" className="serif">{title}</h2>
          {subtitle && <span className="cal-sub">{subtitle}</span>}
        </div>
        <div className="cal-controls">
          <div className="catalog-tabs" role="group" aria-label={t("Affichage")}>
            {VIEWS.map(([id, label]) => <button key={id} type="button" aria-pressed={view === id} onClick={() => setView(id)}>{t(label)}</button>)}
          </div>
          <div className="cal-nav">
            <button type="button" className="cal-round" aria-label={t("Période précédente")} onClick={() => step(-1)}><ChevronLeft size={16} /></button>
            <button type="button" className="cal-today" onClick={goToday}>{t("Aujourd’hui")}</button>
            <button type="button" className="cal-round" aria-label={t("Période suivante")} onClick={() => step(1)}><ChevronRight size={16} /></button>
          </div>
        </div>
      </div>

      {loaded?.error && !loading ? <p role="alert" className="catalog-message">{t("Impossible de charger le calendrier des sorties.")} <button type="button" className="text-action" onClick={() => setRetry((n) => n + 1)}>{t("Réessayer")}</button></p> : null}
      {loaded?.partial && !loading && <p role="status" className="catalog-message">{t("Certaines sorties n’ont pas pu être chargées.")}</p>}

      {view === "week" && (
        <>
          <div className="cal-week" data-loading={loading}>
            {days.map((day) => {
              const list = entriesOf(day);
              const isToday = isSameDay(day, today);
              return (
                <div key={dayKey(day)} className="cal-col" data-today={isToday}>
                  <div className="cal-col-head"><span>{weekday.format(day)}</span><span className="serif">{day.getDate()}</span></div>
                  <div className="cal-col-body">{list.length ? list.map(renderCard) : <div className="cal-col-empty">{t("Aucune sortie")}</div>}</div>
                </div>
              );
            })}
          </div>
          <div className="cal-agenda" data-loading={loading}>
            <div className="cal-strip" role="group" aria-label={t("Jours")}>
              {days.map((day) => (
                <button key={dayKey(day)} type="button" aria-pressed={isSameDay(day, selected)} data-today={isSameDay(day, today)} onClick={() => setSelected(day)}>
                  <span>{weekday.format(day).slice(0, 1)}</span><span className="serif">{day.getDate()}</span>
                  <i data-on={entriesOf(day).length > 0} />
                </button>
              ))}
            </div>
            {dayPanel}
          </div>
        </>
      )}

      {view === "month" && (
        <div className="cal-month-layout" data-loading={loading}>
          <div className="cal-month">
            <div className="cal-month-weekdays">{days.slice(0, 7).map((day) => <span key={dayKey(day)}>{weekday.format(day)}</span>)}</div>
            <div className="cal-month-grid">
              {days.map((day) => {
                const list = entriesOf(day);
                const outside = day.getMonth() !== anchor.getMonth();
                return (
                  <button key={dayKey(day)} type="button" className="cal-cell" data-outside={outside} data-today={isSameDay(day, today)} aria-pressed={isSameDay(day, selected)} aria-label={`${dayMonth.format(day)}, ${list.length}`} onClick={() => setSelected(day)}>
                    <span className="cal-cell-num">{day.getDate()}</span>
                    <span className="cal-cell-minis">
                      {list.slice(0, 3).map((entry) => <span key={`${entry.media_id}-${entry.episode}`} className="cal-mini"><LazyImage src={entry.poster_image} alt="" aspectRatio="" className="cal-thumb-art" /></span>)}
                      {list.length > 3 && <span className="chip cal-more">+{list.length - 3}</span>}
                    </span>
                  </button>
                );
              })}
            </div>
          </div>
          {dayPanel}
        </div>
      )}

      {view === "list" && (
        <div className="cal-list" data-loading={loading}>
          {days.map((day) => {
            const list = entriesOf(day);
            return (
              <div key={dayKey(day)} className="cal-list-day">
                <h3 className="serif" data-today={isSameDay(day, today)}>{weekdayLong.format(day)} <span>{dayMonth.format(day)}</span></h3>
                <div className="cal-rows">{list.length ? list.map(renderRow) : empty}</div>
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}
