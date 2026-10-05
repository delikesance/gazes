/** Prefixes (exact or followed by "/") where the public header and footer must not be rendered. */
const BARE_PREFIXES = ["/admin", "/admin-preview"] as const;

export function isBareChromePath(pathname: string | null | undefined): boolean {
  if (!pathname) return false;
  return BARE_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}
