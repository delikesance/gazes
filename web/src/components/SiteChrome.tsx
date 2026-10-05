"use client";
import type { ReactNode } from "react";
import { usePathname } from "next/navigation";
import { isBareChromePath } from "./SiteChrome.logic";

/** Renders its children (public header or footer) except on the admin panel and its dev preview. */
export function SiteChrome({ children }: { children: ReactNode }) {
  return isBareChromePath(usePathname()) ? null : <>{children}</>;
}
