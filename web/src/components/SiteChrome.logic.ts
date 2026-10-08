/** Prefixes (exact or followed by "/") where the public header and footer must not be rendered. */
const BARE_PREFIXES = ["/admin", "/admin-preview"] as const;

export function isBareChromePath(pathname: string | null | undefined): boolean {
  if (!pathname) return false;
  return BARE_PREFIXES.some((prefix) => pathname === prefix || pathname.startsWith(`${prefix}/`));
}

/**
 * The chrome goes away only once the admin frame is actually on screen: the 404 answered at /admin to anyone
 * the panel does not let in must look exactly like any other 404, public header included.
 */
export function hidesChrome(pathname: string | null | undefined, adminShellMounted: boolean): boolean {
  return adminShellMounted && isBareChromePath(pathname);
}
