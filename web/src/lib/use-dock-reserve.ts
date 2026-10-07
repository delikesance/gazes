"use client";
import { useEffect, type RefObject } from "react";

/**
 * While the dock is visible, shrink the subtitle layer (not the video) so captions stay above it.
 * `remountKey` changes whenever the stage or dock element may have been replaced.
 */
export function useDockReserve(stageRef: RefObject<HTMLDivElement | null>, dockRef: RefObject<HTMLDivElement | null>, remountKey: unknown) {
  useEffect(() => {
    const stage = stageRef.current;
    const dock = dockRef.current;
    if (!stage || !dock) return;
    const update = () => {
      const height = stage.clientHeight;
      const reserve = dock.offsetHeight + (parseFloat(getComputedStyle(dock).bottom) || 0) + 12;
      stage.style.setProperty("--sub-scale", String(height > 0 ? Math.max(0.5, (height - reserve) / height) : 1));
      stage.style.setProperty("--dock-reserve", `${reserve}px`);
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(stage);
    observer.observe(dock);
    return () => observer.disconnect();
  }, [stageRef, dockRef, remountKey]);
}
