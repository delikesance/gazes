"use client";
import { useSyncExternalStore, type ReactNode } from "react";
import { usePathname } from "next/navigation";
import { hidesChrome } from "./SiteChrome.logic";

const noSubscription = () => () => {};

/**
 * Renders its children (public header or footer) except around the admin panel and its dev preview. The server
 * always renders them (it cannot tell the panel from the 404 a stranger gets at the same URL); CSS hides them
 * as soon as `.admin-shell` is in the page, then they unmount on the client (shortcuts, banner).
 */
export function SiteChrome({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const hidden = useSyncExternalStore(noSubscription, () => hidesChrome(pathname, document.querySelector(".admin-shell") !== null), () => false);
  return hidden ? null : <div className="site-chrome">{children}</div>;
}
