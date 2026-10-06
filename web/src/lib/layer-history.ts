"use client";
import { useEffect, useRef } from "react";

/**
 * While a layer (search, sheet) is open it owns one history entry, so the system back gesture closes it
 * instead of leaving the page. Closing it by other means takes that entry off again, unless the page moved on meanwhile.
 */
export function useBackToClose(open: boolean, close: () => void) {
  const closeRef = useRef(close);
  useEffect(() => { closeRef.current = close; });
  useEffect(() => {
    if (!open) return;
    const openedAt = window.location.href;
    window.history.pushState(window.history.state, "", openedAt);
    let popped = false;
    const onPop = () => { popped = true; closeRef.current(); };
    window.addEventListener("popstate", onPop);
    return () => {
      window.removeEventListener("popstate", onPop);
      if (popped) return;
      // A navigation started by the layer (a search) changes the address a moment later: leave that history alone.
      window.setTimeout(() => { if (window.location.href === openedAt) window.history.back(); }, 120);
    };
  }, [open]);
}
