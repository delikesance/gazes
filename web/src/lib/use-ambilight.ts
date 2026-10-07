"use client";
import { useEffect, type RefObject } from "react";

/**
 * Ambilight: paint a tiny copy of the current frame (cover-fit to the stage); CSS blurs and fades it.
 * `remountKey` changes whenever the video or stage element may have been replaced.
 */
export function useAmbilight(enabled: boolean, canvasRef: RefObject<HTMLCanvasElement | null>, videoRef: RefObject<HTMLVideoElement | null>, stageRef: RefObject<HTMLDivElement | null>, remountKey: unknown) {
  useEffect(() => {
    const canvas = canvasRef.current;
    const video = videoRef.current;
    const box = stageRef.current;
    if (!enabled || !canvas || !video || !box) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    let frame = 0;
    let last = 0;
    let stopped = false;
    const draw = () => {
      if (video.readyState < 2 || !video.videoWidth) return;
      const width = 64;
      const height = Math.max(8, Math.round((width * (box.clientHeight || 1)) / (box.clientWidth || 1)));
      if (canvas.width !== width || canvas.height !== height) { canvas.width = width; canvas.height = height; }
      const scale = Math.max(width / video.videoWidth, height / video.videoHeight);
      const w = video.videoWidth * scale;
      const h = video.videoHeight * scale;
      try { ctx.drawImage(video, (width - w) / 2, (height - h) / 2, w, h); } catch { /* frame unavailable */ }
    };
    const loop = (now: number) => {
      if (stopped) return;
      if (now - last >= 100 && !document.hidden && !video.paused) { last = now; draw(); }
      frame = requestAnimationFrame(loop);
    };
    const events = ["loadeddata", "seeked", "pause", "playing"] as const;
    events.forEach((name) => video.addEventListener(name, draw));
    draw();
    frame = requestAnimationFrame(loop);
    return () => {
      stopped = true;
      cancelAnimationFrame(frame);
      events.forEach((name) => video.removeEventListener(name, draw));
    };
  }, [enabled, canvasRef, videoRef, stageRef, remountKey]);
}
