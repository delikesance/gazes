/** Viewer's subtitle preferences: text size as a multiplier, and how far the captions are raised from the bottom edge (percent of the video height). */
export interface SubtitleStyle { scale: number; lift: number }
export const DEFAULT_SUBTITLE_STYLE: SubtitleStyle = { scale: 1, lift: 0 };
export const SUBTITLE_SCALES = [0.8, 1, 1.25, 1.5];
export const SUBTITLE_LIFTS = [0, 5, 10];

const DEFAULT_PLAY_RES_Y = 288; // libass default when a script declares none

/**
 * Rewrites the style table of an ASS script: every style's Fontsize is multiplied by `scale` and
 * its MarginV grows by `lift` percent of PlayResY. Dialogue lines are untouched, so a line forcing
 * its own size (\fs) or position (\pos) keeps it. Text without a V4+ style table is returned as is.
 */
export function styleAss(ass: string, style: SubtitleStyle): string {
  if (style.scale === 1 && style.lift === 0) return ass;
  const lines = ass.split(/\r?\n/);
  const eol = ass.includes("\r\n") ? "\r\n" : "\n";
  const resY = Number(/^PlayResY:\s*(\d+)/im.exec(ass)?.[1]) || DEFAULT_PLAY_RES_Y;
  const liftUnits = Math.round((resY * style.lift) / 100);
  let inStyles = false;
  let fontIdx = -1;
  let marginIdx = -1;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (/^\s*\[.*\]\s*$/.test(line)) { inStyles = /^\s*\[V4\+? Styles\]\s*$/i.test(line); continue; }
    if (!inStyles) continue;
    if (/^Format:/i.test(line)) {
      const columns = line.slice(line.indexOf(":") + 1).split(",").map((c) => c.trim().toLowerCase());
      fontIdx = columns.indexOf("fontsize");
      marginIdx = columns.indexOf("marginv");
      continue;
    }
    if (!/^Style:/i.test(line) || fontIdx < 0) continue;
    const head = line.slice(0, line.indexOf(":") + 1);
    const fields = line.slice(head.length).trimStart().split(",");
    const size = Number(fields[fontIdx]);
    if (Number.isFinite(size) && size > 0) fields[fontIdx] = String(Math.round(size * style.scale * 100) / 100);
    const margin = Number(fields[marginIdx]);
    if (marginIdx >= 0 && Number.isFinite(margin)) fields[marginIdx] = String(margin + liftUnits);
    lines[i] = `${head} ${fields.join(",")}`;
  }
  return lines.join(eol);
}
