/**
 * Sign-in merge of "ma liste". `synced` is the server list this device last saw for this account:
 * an id missing from the server but present there was removed on another device (drop it); one
 * never seen there was added on this device while signed out or offline (upload it); one still on
 * the server and seen there but gone from this device was removed here (remove it on the server).
 */
export function planWatchlistSync(local: number[], remote: number[], synced: number[]) {
  const onServer = new Set(remote);
  const seen = new Set(synced);
  const here = new Set(local);
  return {
    upload: local.filter((id) => !onServer.has(id) && !seen.has(id)),
    keep: local.filter((id) => onServer.has(id) || !seen.has(id)),
    remove: remote.filter((id) => seen.has(id) && !here.has(id)),
  };
}

export function chunks<T>(items: T[], size: number): T[][] {
  const out: T[][] = [];
  for (let i = 0; i < items.length; i += size) out.push(items.slice(i, i + size));
  return out;
}
