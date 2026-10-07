"use client";
import { useCallback, useState } from "react";
import type { SubtitleStyle } from "./ass-style";
import type { AmbilightSettings } from "@/components/PlayerOptionsModal";
import { AMBILIGHT_KEY, RATE_KEY, SUBTITLE_STYLE_KEY, parseAmbilight, parseRate, parseSubtitleStyle, stepRate } from "./player-state";

const storedValue = (key: string) => { try { return typeof window !== "undefined" ? window.localStorage.getItem(key) : null; } catch { return null; } };
const store = (key: string, value: string) => { try { window.localStorage.setItem(key, value); } catch { /* storage unavailable */ } };

/** Viewer settings that outlive an episode: ambilight, subtitle size and lift, playback speed. */
export function usePlayerPreferences() {
  const [ambilight, setAmbilight] = useState<AmbilightSettings>(() => parseAmbilight(storedValue(AMBILIGHT_KEY)));
  const [subtitleStyle, setSubtitleStyle] = useState<SubtitleStyle>(() => parseSubtitleStyle(storedValue(SUBTITLE_STYLE_KEY)));
  const [playbackRate, setPlaybackRateState] = useState(() => parseRate(storedValue(RATE_KEY)));

  const updateAmbilight = useCallback((patch: Partial<AmbilightSettings>) => {
    setAmbilight((current) => {
      const next = { ...current, ...patch };
      store(AMBILIGHT_KEY, JSON.stringify(next));
      return next;
    });
  }, []);

  const updateSubtitleStyle = useCallback((patch: Partial<SubtitleStyle>) => {
    setSubtitleStyle((current) => {
      const next = { ...current, ...patch };
      store(SUBTITLE_STYLE_KEY, JSON.stringify(next));
      return next;
    });
  }, []);

  const setPlaybackRate = useCallback((rate: number) => {
    setPlaybackRateState(rate);
    store(RATE_KEY, String(rate));
  }, []);

  const stepPlaybackRate = useCallback((direction: 1 | -1) => {
    setPlaybackRate(stepRate(playbackRate, direction));
  }, [playbackRate, setPlaybackRate]);

  return { ambilight, updateAmbilight, subtitleStyle, updateSubtitleStyle, playbackRate, setPlaybackRate, stepPlaybackRate };
}
