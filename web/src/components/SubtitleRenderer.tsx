"use client";

import { useEffect, type RefObject } from "react";
import type JASSUB from "jassub";

export function SubtitleRenderer({ videoRef, url, timeOffset, onError }: {
  videoRef: RefObject<HTMLVideoElement | null>;
  url: string;
  timeOffset: number;
  onError: (error: string | null) => void;
}) {
  useEffect(() => {
    const video = videoRef.current;
    const controller = new AbortController();
    let renderer: JASSUB | undefined;
    onError(null);
    if (!video || !url) return;
    async function initialize() {
      try {
        const [{ default: JASSUB }, response] = await Promise.all([
          import("jassub"), fetch(url, { signal: controller.signal }),
        ]);
        if (!response.ok) throw new Error("Subtitle extraction failed");
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
      } catch {
        if (!controller.signal.aborted) {
          onError("Impossible de charger les sous-titres. Désactivez puis resélectionnez la piste pour réessayer.");
          await renderer?.destroy();
        }
      }
    }
    void initialize();
    return () => {
      controller.abort();
      void renderer?.destroy();
    };
  }, [videoRef, url, timeOffset, onError]);
  return null;
}
