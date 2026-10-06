"use client";

import { useEffect, useRef, type RefObject } from "react";
import type JASSUB from "jassub";
import type { PgsRenderer } from "libpgs";
import { carryAss, carryPgs } from "@/lib/subtitle-carry";
import { DEFAULT_SUBTITLE_STYLE, styleAss, type SubtitleStyle } from "@/lib/ass-style";

/** Seconds of captions per regular download, and for the short first one that appears quickly. */
const FULL_WINDOW = 120;
const QUICK_WINDOW = 25;

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

export function SubtitleRenderer({ videoRef, streamKey, url, bitmap = false, timeOffset, style = DEFAULT_SUBTITLE_STYLE, onError }: {
  videoRef: RefObject<HTMLVideoElement | null>;
  /** Changes when the player replaces its video element, including audio switches. */
  streamKey?: string;
  url: string;
  /** The track is a bitmap format (PGS, served raw as .sup) rather than text (ASS). */
  bitmap?: boolean;
  timeOffset: number;
  /** Text size and lift of text (ASS) captions; bitmap tracks cannot be restyled. */
  style?: SubtitleStyle;
  /** Receives a user-facing message and a short machine code, or `null` to clear. */
  onError: (error: string | null, code?: string) => void;
}) {
  const sessionRef = useRef<{ sync: (video: HTMLVideoElement, offset: number) => void } | null>(null);
  const onErrorRef = useRef(onError);
  useEffect(() => { onErrorRef.current = onError; }, [onError]);

  useEffect(() => {
    let video = videoRef.current;
    if (!video || !url) { onErrorRef.current(null); return; }
    const styleScale = style.scale;
    const styleLift = style.lift;
    let offset = 0;
    let disposed = false;
    let renderer: JASSUB | undefined;
    let pgsRenderer: PgsRenderer | undefined;
    let canvas: HTMLCanvasElement | undefined;
    let requestController: AbortController | undefined;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let loadedStart: number | undefined;
    let loadedSpan = FULL_WINDOW;
    let pendingStart: number | undefined;
    let pendingSpan = FULL_WINDOW;
    let failedStart: number | undefined;
    let failedSpan = FULL_WINDOW;
    let applyQueue = Promise.resolve();
    let frameId: number | undefined;
    let width = video.videoWidth;
    let height = video.videoHeight;
    // Keep a bounded set of recent windows for backward and repeated seeks.
    const windows = new Map<number, string | ArrayBuffer>();
    // Seconds covered by each cached window: the first one is short so captions show up fast.
    const spans = new Map<number, number>();
    onErrorRef.current(null);

    const position = () => Math.max(0, video!.currentTime + offset);
    const contains = (start: number | undefined, time: number, span = start === undefined ? FULL_WINDOW : spans.get(start) ?? FULL_WINDOW) =>
      start !== undefined && time >= start && time < start + span;
    function updateVisibility() {
      if (canvas) canvas.style.visibility = contains(loadedStart, position(), loadedSpan) ? "visible" : "hidden";
    }
    function layout() {
      if (!canvas) return;
      // React inserts a replacement video before siblings it does not know, which would put the
      // captions underneath it: keep the canvas directly above the current video.
      if (video!.nextElementSibling !== canvas) video!.insertAdjacentElement("afterend", canvas);
      width = video!.videoWidth || width;
      height = video!.videoHeight || height;
      const scale = Math.min(video!.clientWidth / (width || 1), video!.clientHeight / (height || 1));
      const renderedWidth = width ? width * scale : video!.clientWidth;
      const renderedHeight = height ? height * scale : video!.clientHeight;
      Object.assign(canvas.style, {
        width: `${renderedWidth}px`, height: `${renderedHeight}px`,
        left: `${video!.offsetLeft + (video!.clientWidth - renderedWidth) / 2}px`,
        top: `${video!.offsetTop + (video!.clientHeight - renderedHeight) / 2}px`,
      });
    }
    const observer = new ResizeObserver(() => { layout(); renderCurrent(); });
    function createCanvas() {
      canvas = document.createElement("canvas");
      canvas.style.position = "absolute";
      canvas.style.pointerEvents = "none";
      canvas.style.visibility = "hidden";
      if (bitmap) {
        canvas.style.objectFit = "contain";
      } else {
        canvas.className = "JASSUB";
      }
      video!.insertAdjacentElement("afterend", canvas);
      layout();
      return canvas;
    }
    async function repaint(frame?: VideoFrameCallbackMetadata, force = true) {
      if (disposed) return;
      const target = video;
      const frameOffset = offset;
      updateVisibility();
      if (pgsRenderer) {
        pgsRenderer.renderAtTimestamp(position());
      } else if (renderer) {
        await renderer.ready;
        if (disposed || (frame && (target !== video || frameOffset !== offset))) return;
        // Explicitly render at the new clock even when playback is paused and
        // requestVideoFrameCallback has no frame to deliver after a seek.
        await renderer.manualRender({
          mediaTime: frame?.mediaTime ?? video!.currentTime,
          width: frame?.width || video!.videoWidth || width,
          height: frame?.height || video!.videoHeight || height,
          expectedDisplayTime: performance.now(),
        }, force);
      }
    }
    function renderCurrent(frame?: VideoFrameCallbackMetadata, force = true) {
      void repaint(frame, force).catch(error => {
        if (!disposed) onErrorRef.current("Impossible d’afficher les sous-titres.", subtitleErrorCode(error));
      });
    }
    function nextFrame() {
      const target = video!;
      if (disposed || !target.requestVideoFrameCallback) return;
      frameId = target.requestVideoFrameCallback((_now, frame) => {
        // A callback from a removed video must never use the new seek offset.
        if (disposed || target !== video) return;
        frameId = undefined;
        renderCurrent(frame, false);
        nextFrame();
      });
    }

    async function load(start: number, controller: AbortController, attempt = 0, span = FULL_WINDOW) {
      try {
        let content = (spans.get(start) ?? 0) >= span ? windows.get(start) : undefined;
        if (content === undefined) {
          const chunkURL = new URL(url, window.location.href);
          chunkURL.searchParams.set("start", String(start));
          chunkURL.searchParams.set("duration", String(span));
          const response = await fetch(chunkURL, { signal: controller.signal });
          if (!response.ok) throw new SubtitleError(`SUB_HTTP_${response.status}`);
          content = bitmap ? await response.arrayBuffer() : await response.text();
        }
        if (disposed || controller.signal.aborted) return;
        windows.delete(start);
        windows.set(start, content);
        spans.set(start, Math.max(span, spans.get(start) ?? 0));
        if (windows.size > 4) {
          const oldest = windows.keys().next().value!;
          windows.delete(oldest);
          spans.delete(oldest);
        }

        // Keep the existing canvas and captions while the next window downloads.
        // Decoder updates are serialized, including when a seek supersedes a load.
        applyQueue = applyQueue.catch(() => {}).then(async () => {
          if (disposed || controller.signal.aborted) return;
          // An in-flight decoder update can finish after a seek cancels it.
          // Invalidate the active track before applying so the newest seek
          // restores its cached track instead of trusting the previous one.
          loadedStart = undefined;
          updateVisibility();
          if (bitmap) {
            if (!pgsRenderer) {
              const { PgsRenderer } = await import("libpgs");
              if (disposed || controller.signal.aborted) return;
              // Drive the bitmap clock ourselves so replacing the video does
              // not discard its decoder, canvas, or recently downloaded cues.
              pgsRenderer = new PgsRenderer({ canvas: createCanvas(), workerUrl: "/subtitles/libpgs.worker.js" });
            }
            await pgsRenderer.loadFromBuffer(carryPgs(content as ArrayBuffer, windows.values(), start));
            if (disposed || controller.signal.aborted) return;
            // Reloading can leave the same timestamp index selected: force a repaint.
            pgsRenderer.renderAtTimestamp(-1);
          } else {
            if (!renderer) {
              const { default: JASSUB } = await import("jassub");
              if (disposed || controller.signal.aborted) return;
              renderer = new JASSUB({
                canvas: createCanvas(), subContent: styleAss(carryAss(content as string, windows.values(), start), { scale: styleScale, lift: styleLift }), timeOffset: offset,
                workerUrl: "/subtitles/jassub-worker.js",
                wasmUrl: "/subtitles/jassub-worker.wasm",
                modernWasmUrl: "/subtitles/jassub-worker-modern.wasm",
                // Preloaded, not left to JASSUB's lazy fallback: its fontselect log parser skips italic
                // lookups ("(Family, 400, 100)"), so a track opening on italic cues stayed blank.
                fonts: ["/subtitles/default.woff2"],
                availableFonts: { "liberation sans": "/subtitles/default.woff2" },
                queryFonts: false,
              });
              await renderer.ready;
            } else {
              await renderer.renderer.setTrack(styleAss(carryAss(content as string, windows.values(), start), { scale: styleScale, lift: styleLift }));
            }
          }
          if (disposed || controller.signal.aborted) return;
          loadedStart = start;
          loadedSpan = span;
          await repaint();
        });
        await applyQueue;
        if (disposed || controller.signal.aborted) return;
        pendingStart = undefined;
        failedStart = undefined;
        onErrorRef.current(null);
        // The short first window is on screen: fetch the regular one behind it.
        if (span < FULL_WINDOW) updateWindow();
      } catch (error) {
        if (disposed || controller.signal.aborted) return;
        const transient = error instanceof TypeError || (error instanceof SubtitleError && /^SUB_HTTP_5\d\d$/.test(error.code));
        if (transient && attempt < 2) {
          retryTimer = setTimeout(() => { void load(start, controller, attempt + 1, span); }, 1000 * (attempt + 1));
          return;
        }
        failedStart = start;
        failedSpan = span;
        pendingStart = undefined;
        onErrorRef.current("Impossible de charger les sous-titres. Désactivez puis resélectionnez la piste pour réessayer.", subtitleErrorCode(error));
      }
    }

    function updateWindow(seeking = false) {
      // Thirty seconds of overlap preserve cues already on screen. Advance once
      // a minute, leaving ample captions ahead while the next chunk is fetched.
      const time = position();
      let start = Math.max(0, Math.floor(time / 60) * 60 - 30);
      let span = FULL_WINDOW;
      if (windows.size === 0 && loadedStart === undefined && failedStart === undefined && (pendingStart === undefined || seeking)) {
        // Nothing downloaded yet: the swarm must supply every video byte of a window, so
        // ask for a few seconds around the playhead first to show captions quickly.
        start = Math.max(0, Math.floor(time) - 5);
        span = QUICK_WINDOW;
      } else if (seeking) {
        if (contains(loadedStart, time, loadedSpan)) { start = loadedStart!; span = loadedSpan; }
        else if (!windows.has(start) || (spans.get(start) ?? 0) < span) {
          const cached = [...windows.keys()].reverse().find(candidate => contains(candidate, time));
          if (cached !== undefined) { start = cached; span = spans.get(cached) ?? FULL_WINDOW; }
          else {
            // Seeking into an area with nothing cached: the swarm may have to fetch every byte of
            // a full window, so show captions from a short one first (the full one follows).
            start = Math.max(0, Math.floor(time) - 5);
            span = QUICK_WINDOW;
          }
        }
      }
      updateVisibility();
      if (disposed || (start === pendingStart && span === pendingSpan)) return;
      // Let the short first window finish instead of replacing it with the regular one.
      if (!seeking && pendingStart !== undefined && pendingSpan < FULL_WINDOW && contains(pendingStart, time, pendingSpan)) return;
      if (pendingStart !== undefined) {
        requestController?.abort();
        clearTimeout(retryTimer);
        pendingStart = undefined;
      }
      if (start === loadedStart && span <= loadedSpan) return;
      if (start === failedStart && span === failedSpan) return;
      requestController = new AbortController();
      pendingStart = start;
      pendingSpan = span;
      void load(start, requestController, 0, span);
    }

    function onTimeUpdate() { updateWindow(); renderCurrent(undefined, false); }
    function onSeek() { layout(); updateWindow(true); renderCurrent(); }
    function bind(target: HTMLVideoElement, attach: boolean) {
      for (const [event, handler] of [
        ["timeupdate", onTimeUpdate], ["seeking", onSeek], ["seeked", onSeek],
        ["loadedmetadata", onSeek], ["canplay", onSeek],
      ] as const) {
        if (attach) target.addEventListener(event, handler);
        else target.removeEventListener(event, handler);
      }
      if (attach) { observer.observe(target); nextFrame(); }
      else {
        observer.unobserve(target);
        if (frameId !== undefined) target.cancelVideoFrameCallback?.(frameId);
        frameId = undefined;
      }
    }
    const session = {
      sync(target: HTMLVideoElement, nextOffset: number) {
        const replaced = target !== video;
        if (replaced) { bind(video!, false); video = target; bind(target, true); }
        offset = nextOffset;
        failedStart = undefined;
        if (renderer) renderer.timeOffset = offset;
        layout();
        updateWindow(true);
        renderCurrent();
      },
    };
    sessionRef.current = session;
    bind(video, true);
    return () => {
      disposed = true;
      if (sessionRef.current === session) sessionRef.current = null;
      requestController?.abort();
      clearTimeout(retryTimer);
      bind(video!, false);
      observer.disconnect();
      pgsRenderer?.dispose();
      void renderer?.destroy();
      canvas?.remove();
      windows.clear();
      spans.clear();
    };
  }, [videoRef, url, bitmap, style.scale, style.lift]);
  useEffect(() => {
    if (videoRef.current) sessionRef.current?.sync(videoRef.current, timeOffset);
  }, [videoRef, streamKey, timeOffset, url, bitmap]);
  return null;
}
