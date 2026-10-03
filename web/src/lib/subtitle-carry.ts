function assTime(value: string) {
  const match = value.match(/^(\d+):(\d+):(\d+(?:\.\d+)?)$/);
  return match ? Number(match[1]) * 3600 + Number(match[2]) * 60 + Number(match[3]) : NaN;
}

/** Preserve known dialogues that started in an older window and are still active. */
export function carryAss(content: string, previous: Iterable<string | ArrayBuffer>, start: number): string {
  const seen = new Set(content.split(/\r?\n/));
  const carry: string[] = [];
  for (const window of previous) {
    if (typeof window !== 'string') continue;
    for (const line of window.split(/\r?\n/)) {
      const match = line.match(/^Dialogue:\s*[^,]+,([^,]+),([^,]+),/);
      if (match && assTime(match[1]) < start && assTime(match[2]) > start && !seen.has(line)) { carry.push(line); seen.add(line); }
    }
  }
  return carry.length ? content.trimEnd() + '\n' + carry.join('\n') + '\n' : content;
}

/** Rebuild the last complete PGS epoch, including palette/object definitions.
 * Loading only the new window loses an image whose display began earlier. */
export function carryPgs(content: ArrayBuffer, previous: Iterable<string | ArrayBuffer>, start: number): ArrayBuffer {
  let selected: Uint8Array | undefined;
  let latest = -1;
  for (const window of previous) {
    if (!(window instanceof ArrayBuffer) || window === content) continue;
    const bytes = new Uint8Array(window), view = new DataView(window);
    let epoch = -1, end = -1, last = -1;
    for (let pos = 0; pos + 13 <= bytes.length;) {
      if (bytes[pos] !== 0x50 || bytes[pos + 1] !== 0x47) break;
      const length = view.getUint16(pos + 11), next = pos + 13 + length;
      if (next > bytes.length) break;
      const time = view.getUint32(pos + 2) / 90000;
      if (time > start) break;
      if (bytes[pos + 10] === 0x16 && length >= 8 && bytes[pos + 20] === 0x80) epoch = pos;
      if (bytes[pos + 10] === 0x80 && epoch >= 0) { end = next; last = time; }
      pos = next;
    }
    if (epoch >= 0 && end > epoch && last > latest) { selected = bytes.subarray(epoch, end); latest = last; }
  }
  if (!selected) return content;
  const result = new Uint8Array(selected.length + content.byteLength);
  result.set(selected); result.set(new Uint8Array(content), selected.length);
  return result.buffer;
}
