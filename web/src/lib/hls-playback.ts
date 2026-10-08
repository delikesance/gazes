import Hls from 'hls.js';
import { getApiBase } from './api';
import { diagnosticEvent, diagnosticHeaders, type PlaybackDiagnostic } from './diagnostics';
import type { VideoMetadata } from '../types/api';

export type PlaybackPhase = 'preparing' | 'ready' | 'seeking' | 'buffering' | 'paused' | 'ended' | 'failed';
export interface PlaybackSession {
  id: string; duration: number; timeline_origin: number; playlist_url: string;
  metadata: VideoMetadata; position: number; generation: number;
}
export interface PlaybackSnapshot { phase: PlaybackPhase; position: number; duration: number; playing: boolean; generation: number }
interface Callbacks {
  state: (state: PlaybackSnapshot) => void;
  metadata: (metadata: VideoMetadata, origin: number) => void;
  error: (message: string) => void;
  gesture: () => void;
}

// The packager supplies a fixed one-second transport epoch. Letting hls.js
// infer it from the first DTS would move presentation time by the B-frame
// decode lead, and choosing an audio DTS could move it by encoder priming.
class EpisodeStreamController extends Hls.DefaultConfig.streamController {
  protected onManifestLoading() {
    super.onManifestLoading();
    this.initPTS[0] = { baseTime: 90_000, timescale: 90_000, trackId: 1 };
  }
}

/** Owns the media lifetime. React never rebases or replaces the video on seek. */
/** Whether this browser decodes AV1; when it cannot, the server transcodes AV1 sources to H.264. */
function canDecodeAV1() { return typeof MediaSource !== 'undefined' && typeof MediaSource.isTypeSupported === 'function' && MediaSource.isTypeSupported('video/mp4; codecs="av01.0.08M.10"'); }

export class HlsPlaybackController {
  private hls?: Hls;
  private session?: PlaybackSession;
  private request?: AbortController;
  private heartbeat?: ReturnType<typeof setInterval>;
  private disposed = false;
  private revision = 0;
  private desired = 0;
  private seekAt = 0;
  private lastUseful = performance.now();
  private lastPosition = 0;
  private lastBuffer = 0;
  private recovery = false;
  private loadingStopped = false;
  private snapshot: PlaybackSnapshot = { phase: 'preparing', position: 0, duration: 0, playing: true, generation: 0 };
  private listeners: Array<[string, EventListener]> = [];

  constructor(private video: HTMLVideoElement, private callbacks: Callbacks, private diagnostic?: PlaybackDiagnostic, paused = false) {
    this.snapshot.playing = !paused;
    const bind = (event: string, fn: () => void) => { const handler = fn as EventListener; video.addEventListener(event, handler); this.listeners.push([event, handler]); };
    bind('timeupdate', () => this.progress());
    bind('progress', () => this.progress());
    bind('waiting', () => { if (this.snapshot.phase !== 'seeking' && this.snapshot.phase !== 'preparing') this.emit('buffering'); });
    bind('playing', () => this.settled());
    bind('seeked', () => this.settled());
    bind('canplay', () => { this.settled(); void this.resume(); });
    bind('ended', () => this.emit('ended'));
    bind('error', () => { if (!this.hls && this.video.error) this.fail('Cette source ne fournit pas un flux HLS compatible.'); });
    this.heartbeat = setInterval(() => {
      if (this.disposed || !this.session) return;
      const position = ['seeking', 'preparing'].includes(this.snapshot.phase) ? this.desired : this.video.currentTime;
      void this.update(position).catch(() => {});
      if (this.snapshot.playing && this.snapshot.phase !== 'ended' && performance.now() - this.lastUseful > 30_000) this.fail('Cette source ne fournit plus de segments pour la position demandée.');
    }, 1000);
  }

  private emit(phase = this.snapshot.phase) {
    if (this.disposed) return;
    this.snapshot = { ...this.snapshot, phase, position: phase === 'seeking' || phase === 'preparing' ? this.desired : this.video.currentTime };
    this.callbacks.state({ ...this.snapshot });
  }
  private fail(message: string) { if (this.disposed || this.snapshot.phase === 'failed') return; this.emit('failed'); this.callbacks.error(message); }

  async open(infoHash: string, fileIndex: number, audioTrack: number, position: number) {
    const revision = ++this.revision;
    this.request?.abort(); this.hls?.destroy(); this.hls = undefined;
    const previous = this.session; this.session = undefined;
    if (previous) void this.deleteSession(previous.id);
    this.request = new AbortController();
    this.video.pause(); this.video.removeAttribute('src'); this.video.load();
    this.desired = position; this.lastUseful = performance.now(); this.lastBuffer = 0; this.lastPosition = position;
    this.snapshot = { ...this.snapshot, phase: 'preparing', position, generation: this.snapshot.generation + 1 };
    this.callbacks.state({ ...this.snapshot });
    try {
      const response = await fetch(`${getApiBase()}/playback/sessions`, {
        method: 'POST', headers: { 'Content-Type': 'application/json', ...diagnosticHeaders(this.diagnostic) }, signal: this.request.signal,
        body: JSON.stringify({ info_hash: infoHash, file_index: fileIndex, audio_track: audioTrack, position, no_av1: !canDecodeAV1(), replaces: previous?.id }),
      });
      if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error === 'seek_index_unavailable' ? 'Cette source ne possède pas d’index permettant une lecture fiable.' : body.error === 'stream_limit_reached' ? 'Le service est complet pour le moment. Réessayez dans quelques minutes.' : 'Impossible de préparer cette source pour la lecture.'); }
      const session = await response.json() as PlaybackSession;
      if (this.disposed || revision !== this.revision) { void this.deleteSession(session.id); return; }
      this.session = session; this.snapshot.duration = session.duration;
      this.callbacks.metadata({ ...session.metadata, duration_sec: session.duration }, session.timeline_origin);
      await this.update(this.desired);
      if (this.disposed || revision !== this.revision) return;
      const playlist = new URL(session.playlist_url, window.location.href).href;
      const safari = /AppleWebKit/.test(navigator.userAgent) && !/(Chrome|Chromium|CriOS|Edg|OPR|Android|Firefox|FxiOS)/.test(navigator.userAgent);
      if (this.video.canPlayType('application/vnd.apple.mpegurl') && (safari || !Hls.isSupported())) {
        const ready = () => { if (revision !== this.revision) return; this.video.currentTime = this.desired; void this.resume(); };
        this.video.addEventListener('loadedmetadata', ready, { once: true }); this.listeners.push(['loadedmetadata', ready]);
        this.video.src = playlist;
      } else if (Hls.isSupported()) {
        const hls = new Hls({ streamController: EpisodeStreamController, debug: new URLSearchParams(location.search).has('debugHls'), startPosition: this.desired, maxBufferLength: 30, maxMaxBufferLength: 30, backBufferLength: 10, enableWorker: false,
          fragLoadPolicy: { default: { maxTimeToFirstByteMs: 27_000, maxLoadTimeMs: 30_000, timeoutRetry: { maxNumRetry: 1, retryDelayMs: 1000, maxRetryDelayMs: 2000 }, errorRetry: { maxNumRetry: 2, retryDelayMs: 1000, maxRetryDelayMs: 2000 } } },
        });
        this.hls = hls;
        hls.on(Hls.Events.ERROR, (_event, data) => {
          if (this.disposed || this.hls !== hls || !data.fatal) return;
          diagnosticEvent(this.diagnostic, 'playback.hls_error', { error_code: data.details, generation: this.snapshot.generation, position: this.desired });
          if (data.type === Hls.ErrorTypes.MEDIA_ERROR && !this.recovery) { this.recovery = true; hls.recoverMediaError(); return; }
          this.fail('Cette source ne fournit pas de segments vidéo compatibles.');
        });
        hls.on(Hls.Events.MANIFEST_PARSED, () => { if (this.hls === hls) void this.resume(); });
        hls.on(Hls.Events.FRAG_BUFFERED, () => { if (this.hls === hls) { this.progress(); this.settled(); } });
        hls.attachMedia(this.video); hls.loadSource(playlist);
      } else this.fail('Ce navigateur ne prend pas en charge la lecture de cette source.');
    } catch (error) {
      if (this.disposed || revision !== this.revision || this.request?.signal.aborted) return;
      this.fail(error instanceof Error ? error.message : 'Impossible de préparer cette source pour la lecture.');
    }
  }

  seek(position: number) {
    if (this.disposed) return;
    position = Math.max(0, Math.min(position, this.snapshot.duration || position));
    this.desired = position; this.snapshot.generation++; this.seekAt = performance.now(); this.lastUseful = this.seekAt;
    this.snapshot.phase = 'seeking'; this.emit('seeking');
    const generation = this.snapshot.generation;
    diagnosticEvent(this.diagnostic, 'playback.seek_started', { position, generation });
    let buffered = false;
    for (let n = 0; n < this.video.buffered.length; n++) if (position >= this.video.buffered.start(n) && position < this.video.buffered.end(n) - 0.05) buffered = true;
    if (!buffered && this.hls) { this.hls.stopLoad(); this.loadingStopped = true; }
    if (this.video.readyState > 0) this.video.currentTime = position;
    void this.update(position).then(() => {
      if (this.disposed || generation !== this.snapshot.generation) return;
      if (!buffered || this.loadingStopped) { this.hls?.startLoad(position, true); this.loadingStopped = false; }
      this.settled();
    }).catch(() => { if (!this.disposed && generation === this.snapshot.generation) this.fail('Impossible de préparer les segments à cette position.'); });
  }
  private settled() {
    if (this.disposed || this.snapshot.phase === 'preparing' && !this.session) return;
    if (this.video.readyState < 2 || Math.abs(this.video.currentTime - this.desired) > 0.08 && ['seeking', 'preparing'].includes(this.snapshot.phase)) return;
    if (this.snapshot.phase === 'seeking') diagnosticEvent(this.diagnostic, 'playback.seek_completed', { position: this.video.currentTime, target: this.desired, generation: this.snapshot.generation, duration_ms: performance.now() - this.seekAt });
    this.lastUseful = performance.now(); this.emit(this.snapshot.playing ? 'ready' : 'paused');
  }
  private progress() {
    const position = this.video.currentTime;
    let end = position;
    for (let n = 0; n < this.video.buffered.length; n++) if (position >= this.video.buffered.start(n) - 0.05 && position <= this.video.buffered.end(n)) end = this.video.buffered.end(n);
    if (position > this.lastPosition + 0.01 || end > this.lastBuffer + 0.01) this.lastUseful = performance.now();
    this.lastPosition = position; this.lastBuffer = end;
    if (!['seeking', 'preparing'].includes(this.snapshot.phase)) this.desired = position;
    this.emit();
  }
  private lastHeartbeat = 0;
  private async update(position: number) {
    if (!this.session) return;
    const now = performance.now();
    if (now - this.lastHeartbeat < 15_000 && this.snapshot.phase !== 'seeking' && this.session.generation === this.snapshot.generation) return;
    const session = this.session; const generation = this.snapshot.generation; this.lastHeartbeat = now;
    const response = await fetch(`${getApiBase()}/playback/sessions/${session.id}`, { method: 'PUT', headers: { 'Content-Type': 'application/json', ...diagnosticHeaders(this.diagnostic) }, body: JSON.stringify({ position, generation }), signal: this.request?.signal });
    if (!response.ok) throw new Error('session_unavailable');
    if (this.session === session) session.generation = Math.max(session.generation, generation);
  }
  private async resume() {
    if (!this.snapshot.playing || this.disposed) return;
    try { await this.video.play(); } catch (error) {
      if (this.disposed || error instanceof DOMException && error.name === 'AbortError') return;
      if (error instanceof DOMException && error.name === 'NotAllowedError') { this.snapshot.playing = false; this.emit('paused'); this.callbacks.gesture(); return; }
      this.fail('Impossible de reprendre la lecture.');
    }
  }
  play() { this.snapshot.playing = true; this.lastUseful = performance.now(); void this.resume(); }
  pause() { this.snapshot.playing = false; this.video.pause(); this.emit(['preparing', 'seeking'].includes(this.snapshot.phase) ? this.snapshot.phase : 'paused'); }
  get position() { return ['preparing','seeking','failed'].includes(this.snapshot.phase) ? this.desired : this.video.currentTime; }
  private async deleteSession(id: string) { await fetch(`${getApiBase()}/playback/sessions/${id}`, { method: 'DELETE', keepalive: true }).catch(() => {}); }
  dispose() {
    this.disposed = true; this.revision++; this.request?.abort(); clearInterval(this.heartbeat);
    this.hls?.destroy(); this.video.pause(); this.video.removeAttribute('src'); this.video.load();
    for (const [event, listener] of this.listeners) this.video.removeEventListener(event, listener);
    if (this.session) void this.deleteSession(this.session.id);
  }
}
