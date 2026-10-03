"use client";
import { useEffect, useState } from 'react';
import { getApiBase } from './api';
export type PlaybackEngine = 'legacy' | 'hls';
let configuration: Promise<PlaybackEngine> | undefined;
export function usePlaybackEngine() {
  const [engine, setEngine] = useState<PlaybackEngine | null>(null);
  useEffect(() => {
    let active = true;
    const override = new URLSearchParams(window.location.search).get('player');
    if (override === 'hls' || override === 'legacy') {
      queueMicrotask(() => { if (active) setEngine(override); });
      return () => { active = false; };
    }
    configuration ??= fetch(`${getApiBase()}/playback/config`).then(async response => response.ok && (await response.json()).engine === 'hls' ? 'hls' : 'legacy').catch(() => 'legacy');
    void configuration.then(value => { if (active) setEngine(value); });
    return () => { active = false; };
  }, []);
  return engine;
}
