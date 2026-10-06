/** Largest subtitle file accepted from the viewer (a full episode is a few hundred KB). */
export const MAX_SUBTITLE_FILE_BYTES = 2 * 1024 * 1024;

const ASS_HEADER = [
  "[Script Info]", "ScriptType: v4.00+", "PlayResX: 1280", "PlayResY: 720", "WrapStyle: 0", "ScaledBorderAndShadow: yes", "",
  "[V4+ Styles]",
  "Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding",
  "Style: Default,Liberation Sans,48,&H00FFFFFF,&H000000FF,&H00000000,&H64000000,0,0,0,0,100,100,0,0,1,2,1,2,40,40,40,1", "",
  "[Events]", "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
];

/** Seconds to the ASS H:MM:SS.cc form. */
function assTime(seconds: number): string {
  const cs = Math.round(Math.max(0, seconds) * 100);
  const h = Math.floor(cs / 360000), m = Math.floor((cs % 360000) / 6000), s = Math.floor((cs % 6000) / 100);
  return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}.${String(cs % 100).padStart(2, "0")}`;
}

/** "00:01:02,500", "01:02.500" or "1:02:03.5" to seconds; NaN when it is not a timestamp. */
export function parseCueTime(raw: string): number {
  const match = /^(?:(\d+):)?(\d{1,2}):(\d{1,2})(?:[.,](\d{1,3}))?$/.exec(raw.trim());
  if (!match) return NaN;
  const fraction = match[4] ? Number(`0.${match[4]}`) : 0;
  return Number(match[1] ?? 0) * 3600 + Number(match[2]) * 60 + Number(match[3]) + fraction;
}

/** Cue text (SRT/WebVTT) to ASS: line breaks and the basic i/b/u tags survive, everything else is dropped. */
function cueText(lines: string[]): string {
  return lines.join("\\N")
    .replace(/\{\\an\d\}/g, "")
    // Braces would open ASS override blocks: neutralise them before the real tags are added.
    .replace(/\{/g, "(")
    .replace(/\}/g, ")")
    .replace(/<\s*(i|b|u)\s*>/gi, (_m, tag: string) => `{\\${tag.toLowerCase()}1}`)
    .replace(/<\s*\/\s*(i|b|u)\s*>/gi, (_m, tag: string) => `{\\${tag.toLowerCase()}0}`)
    .replace(/<[^>]*>/g, "")
    .trim();
}

/**
 * Turns a subtitle file chosen by the viewer into ASS text the player can render: ASS/SSA are kept
 * as is, SRT and WebVTT are converted. Returns null when the file is none of those or holds no cue.
 */
export function subtitleFileToAss(name: string, text: string): string | null {
  const clean = text.replace(/^﻿/, "");
  const lower = name.toLowerCase();
  if (lower.endsWith(".ass") || lower.endsWith(".ssa")) return /^\s*\[Events\]/im.test(clean) || /\n\s*\[Events\]/i.test(clean) ? clean : null;
  if (!lower.endsWith(".srt") && !lower.endsWith(".vtt")) return null;
  const events: string[] = [];
  for (const block of clean.split(/\r?\n\s*\r?\n/)) {
    const lines = block.split(/\r?\n/).filter((l) => l.trim() !== "");
    const at = lines.findIndex((l) => l.includes("-->"));
    if (at < 0) continue;
    const [startRaw, endRaw] = lines[at].split("-->");
    const start = parseCueTime(startRaw), end = parseCueTime(endRaw.trim().split(/\s+/)[0]);
    const body = cueText(lines.slice(at + 1));
    if (Number.isFinite(start) && Number.isFinite(end) && end > start && body) events.push(`Dialogue: 0,${assTime(start)},${assTime(end)},Default,,0,0,0,,${body}`);
  }
  return events.length ? [...ASS_HEADER, ...events, ""].join("\n") : null;
}
