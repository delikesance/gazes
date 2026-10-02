"use client";

import { useEffect, type RefObject } from "react";
import type JASSUB from "jassub";
import type { PgsRenderer } from "libpgs";

class SubtitleError extends Error {
  constructor(public code: string) { super(code); }
}

/** Short, greppable code for the failure: HTTP status, network, or the renderer's error name. */
function subtitleErrorCode(error: unknown): string {
  if (error instanceof SubtitleError) return error.code;
  if (error instanceof TypeError) return "SUB_NETWORK";
  if (error instanceof Error && error.name && error.name !== "Error") return `SUB_RENDER_${error.name.toUpperCase()}`;
  return "SUB_RENDER_FAILED";
}

export function SubtitleRenderer({ videoRef, url, bitmap = false, timeOffset, onError }: {
  videoRef: RefObject<HTMLVideoElement | null>;
  url: string;
  /** The track is a bitmap format (PGS, served raw as .sup) rather than text (ASS). */
  bitmap?: boolean;
  timeOffset: number;
  /** Receives a user-facing message and a short machine code, or `null` to clear. */
  onError: (error: string | null, code?: string) => void;
}) {
  useEffect(() => {
    const video = videoRef.current;
    if (!video || !url) { onError(null); return; }
    let disposed = false;
    let renderer: JASSUB | undefined;
    let pgsRenderer: PgsRenderer | undefined;
    let requestController: AbortController | undefined;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let loadedStart: number | undefined;
    let pendingStart: number | undefined;
    let failedStart: number | undefined;
    let applyQueue = Promise.resolve();
    onError(null);

    async function load(start: number, controller: AbortController, attempt = 0) {
      try {
        const chunkURL = new URL(url, window.location.href);
        chunkURL.searchParams.set("start", String(start));
        chunkURL.searchParams.set("duration", "120");
        const response = await fetch(chunkURL, { signal: controller.signal });
        if (!response.ok) throw new SubtitleError(`SUB_HTTP_${response.status}`);
        const content = bitmap ? await response.arrayBuffer() : await response.text();
        if (disposed || controller.signal.aborted) return;

        // Keep the existing canvas and captions while the next window downloads.
        // Decoder updates are serialized, including when a seek supersedes a load.
        applyQueue = applyQueue.catch(() => {}).then(async () => {
          if (disposed || controller.signal.aborted) return;
          if (bitmap) {
            if (!pgsRenderer) {
              const { PgsRenderer } = await import("libpgs");
              if (disposed || controller.signal.aborted) return;
              pgsRenderer = new PgsRenderer({ video: video!, timeOffset, workerUrl: "/subtitles/libpgs.worker.js" });
            }
            await pgsRenderer.loadFromBuffer(content as ArrayBuffer);
            if (disposed || controller.signal.aborted) return;
            // Reloading can leave the same timestamp index selected: force a repaint.
            pgsRenderer.renderAtTimestamp(-1);
            pgsRenderer.renderAtTimestamp(video!.currentTime + timeOffset);
          } else {
            if (!renderer) {
              const { default: JASSUB } = await import("jassub");
              if (disposed || controller.signal.aborted) return;
              renderer = new JASSUB({
                video: video!, subContent: content as string, timeOffset,
                workerUrl: "/subtitles/jassub-worker.js",
                wasmUrl: "/subtitles/jassub-worker.wasm",
                modernWasmUrl: "/subtitles/jassub-worker-modern.wasm",
                availableFonts: { "liberation sans": "/subtitles/default.woff2" },
                queryFonts: false,
              });
              await renderer.ready;
            } else {
              await renderer.renderer.setTrack(content as string);
              await renderer.resize(true);
            }
          }
        });
        await applyQueue;
        if (disposed || controller.signal.aborted) return;
        loadedStart = start;
        pendingStart = undefined;
        failedStart = undefined;
        onError(null);
      } catch (error) {
        if (disposed || controller.signal.aborted) return;
        const transient = error instanceof TypeError || (error instanceof SubtitleError && /^SUB_HTTP_5\d\d$/.test(error.code));
        if (transient && attempt < 2) {
          retryTimer = setTimeout(() => { void load(start, controller, attempt + 1); }, 1000 * (attempt + 1));
          return;
        }
        failedStart = start;
        pendingStart = undefined;
        onError("Impossible de charger les sous-titres. Désactivez puis resélectionnez la piste pour réessayer.", subtitleErrorCode(error));
      }
    }

    function updateWindow() {
      // Thirty seconds of overlap preserve cues already on screen. Advance once
      // a minute, leaving ample captions ahead while the next chunk is fetched.
      const position = Math.max(0, video!.currentTime + timeOffset);
      const start = Math.max(0, Math.floor(position / 60) * 60 - 30);
      if (disposed || start === pendingStart) return;
      if (pendingStart !== undefined) {
        requestController?.abort();
        clearTimeout(retryTimer);
        pendingStart = undefined;
      }
      if (start === loadedStart || start === failedStart) return;
      requestController = new AbortController();
      pendingStart = start;
      void load(start, requestController);
    }

    video.addEventListener("timeupdate", updateWindow);
    video.addEventListener("seeked", updateWindow);
    updateWindow();
    return () => {
      disposed = true;
      requestController?.abort();
      clearTimeout(retryTimer);
      video.removeEventListener("timeupdate", updateWindow);
      video.removeEventListener("seeked", updateWindow);
      pgsRenderer?.dispose();
      void renderer?.destroy();
    };
  }, [videoRef, url, bitmap, timeOffset, onError]);
  return null;
}
