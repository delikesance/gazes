"use client";

import { useEffect, type RefObject } from "react";
import type JASSUB from "jassub";

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
    const controller = new AbortController();
    let renderer: JASSUB | undefined;
    let pgsRenderer: { dispose(): void } | undefined;
    let blobUrl: string | undefined;
    onError(null);
    if (!video || !url) return;
    async function initialize() {
      try {
        if (bitmap) {
          // One extraction only: fetch the raw .sup ourselves and hand libpgs a blob URL.
          const [{ PgsRenderer }, response] = await Promise.all([
            import("libpgs"), fetch(url, { signal: controller.signal }),
          ]);
          if (!response.ok) throw new SubtitleError(`SUB_HTTP_${response.status}`);
          const blob = await response.blob();
          if (controller.signal.aborted) return;
          blobUrl = URL.createObjectURL(blob);
          const pgs = new PgsRenderer({ video: video!, subUrl: blobUrl, timeOffset, workerUrl: "/subtitles/libpgs.worker.js" });
          pgsRenderer = pgs;
          await pgs.ready;
          return;
        }
        const [{ default: JASSUB }, response] = await Promise.all([
          import("jassub"), fetch(url, { signal: controller.signal }),
        ]);
        if (!response.ok) throw new SubtitleError(`SUB_HTTP_${response.status}`);
        const subContent = await response.text();
        if (controller.signal.aborted) return;
        renderer = new JASSUB({
          video: video!, subContent, timeOffset,
          workerUrl: "/subtitles/jassub-worker.js",
          wasmUrl: "/subtitles/jassub-worker.wasm",
          modernWasmUrl: "/subtitles/jassub-worker-modern.wasm",
          availableFonts: { "liberation sans": "/subtitles/default.woff2" },
          queryFonts: false,
        });
        await renderer.ready;
      } catch (error) {
        if (!controller.signal.aborted) {
          onError("Impossible de charger les sous-titres. Désactivez puis resélectionnez la piste pour réessayer.", subtitleErrorCode(error));
          pgsRenderer?.dispose();
          await renderer?.destroy();
        }
      }
    }
    void initialize();
    return () => {
      controller.abort();
      pgsRenderer?.dispose();
      if (blobUrl) URL.revokeObjectURL(blobUrl);
      void renderer?.destroy();
    };
  }, [videoRef, url, bitmap, timeOffset, onError]);
  return null;
}
