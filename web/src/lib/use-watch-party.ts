"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { getApiBase } from "./api";
import { randomId } from "./random-id";

type PartyAction = "play" | "pause" | "seek";
const SUPPRESS_MS = 1500;
const DRIFT_SECONDS = 2;

function roomFromUrl(): string | null {
  const room = new URLSearchParams(window.location.search).get("party");
  return room && /^[a-z0-9]{6,32}$/.test(room) ? room : null;
}
function newRoom(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(8));
  return Array.from(bytes, (b) => (b % 36).toString(36)).join("");
}

/**
 * Watch party: viewers sharing `?party=<room>` mirror each other's play, pause and seek
 * through the backend's event relay. Remote actions never echo back.
 */
export function useWatchParty({ isPlaying, getPosition, togglePlay, seek }: {
  isPlaying: boolean;
  getPosition: () => number;
  togglePlay: () => void;
  seek: (seconds: number) => void;
}) {
  const [room, setRoom] = useState<string | null>(() => (typeof window === "undefined" ? null : roomFromUrl()));
  const me = useRef("");
  const quietUntil = useRef(0);
  const live = useRef({ isPlaying, getPosition, togglePlay, seek });
  const lastPlaying = useRef(isPlaying);

  useEffect(() => { live.current = { isPlaying, getPosition, togglePlay, seek }; });
  useEffect(() => { me.current = randomId().replaceAll("-", "").slice(0, 16); }, []);

  const send = useCallback((type: PartyAction, t: number) => {
    if (!room || Date.now() < quietUntil.current) return;
    void fetch(`${getApiBase()}/party/${room}/events`, {
      method: "POST", headers: { "Content-Type": "application/json" }, keepalive: true,
      body: JSON.stringify({ from: me.current, type, t: Math.max(0, t) }),
    }).catch(() => {});
  }, [room]);

  useEffect(() => {
    if (!room) return;
    const source = new EventSource(`${getApiBase()}/party/${room}/events?from=${me.current}`);
    source.onmessage = (message) => {
      let event: { type: PartyAction; t: number };
      try { event = JSON.parse(message.data); } catch { return; }
      quietUntil.current = Date.now() + SUPPRESS_MS;
      const state = live.current;
      if (event.type === "seek" || Math.abs(state.getPosition() - event.t) > DRIFT_SECONDS) state.seek(event.t);
      if (event.type === "play" && !state.isPlaying) state.togglePlay();
      if (event.type === "pause" && state.isPlaying) state.togglePlay();
    };
    return () => source.close();
  }, [room]);

  useEffect(() => {
    if (lastPlaying.current === isPlaying) return;
    lastPlaying.current = isPlaying;
    send(isPlaying ? "play" : "pause", live.current.getPosition());
  }, [isPlaying, send]);

  /** Starts a party on this page (or reuses the current one) and returns the link to share. */
  const invite = useCallback(() => {
    const code = room ?? newRoom();
    const url = new URL(window.location.href);
    url.searchParams.set("party", code);
    url.searchParams.delete("t");
    if (!room) { window.history.replaceState(null, "", url); setRoom(code); }
    return url.toString();
  }, [room]);

  return { room, send, invite };
}
