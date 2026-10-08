import { test } from 'node:test';
import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { build } from 'esbuild';

// player-state.ts imports sibling modules without extensions: bundle it rather than strip types.
const bundled = await build({ entryPoints: [resolve(import.meta.dirname, '../src/lib/player-state.ts')], bundle: true, write: false, format: 'esm', platform: 'neutral' });
const S = await import(`data:text/javascript;base64,${Buffer.from(bundled.outputFiles[0].text).toString('base64')}`);

const timeouts = { metadata: 30, startup: 20, stall: 15, max: 100 };
const track = (index, language, title = '', extra = {}) => ({ index, language, title, codec: 'ass', is_default: false, ...extra });

test('formatTime pads minutes and seconds, adds hours only when needed', () => {
  assert.equal(S.formatTime(0), '00:00');
  assert.equal(S.formatTime(NaN), '00:00');
  assert.equal(S.formatTime(-4), '00:00');
  assert.equal(S.formatTime(65.9), '01:05');
  assert.equal(S.formatTime(3600 + 5 * 60 + 7), '1:05:07');
});

test('preferences parse stored values and fall back field by field', () => {
  assert.deepEqual(S.parseSubtitleStyle(null), { scale: 1, lift: 0 });
  assert.deepEqual(S.parseSubtitleStyle('{"scale":1.5,"lift":99}'), { scale: 1.5, lift: 0 });
  assert.deepEqual(S.parseSubtitleStyle('not json'), { scale: 1, lift: 0 });
  assert.deepEqual(S.parseAmbilight(null), S.AMBILIGHT_DEFAULT);
  assert.deepEqual(S.parseAmbilight('{"on":false}'), { ...S.AMBILIGHT_DEFAULT, on: false });
  assert.deepEqual(S.parseAmbilight('{'), S.AMBILIGHT_DEFAULT);
  assert.equal(S.parseRate('1.5'), 1.5);
  assert.equal(S.parseRate('3'), 1);
  assert.equal(S.parseRate(null), 1);
});

test('stepRate moves one notch and stops at both ends', () => {
  assert.equal(S.stepRate(1, 1), 1.25);
  assert.equal(S.stepRate(1, -1), 0.75);
  assert.equal(S.stepRate(2, 1), 2);
  assert.equal(S.stepRate(0.5, -1), 0.5);
});

test('subtitle tracks: unsupported bitmap codecs are hidden, only PGS renders as bitmap', () => {
  const tracks = [track(0, 'fre', '', { codec: 'dvd_subtitle' }), track(1, 'eng', '', { codec: 'hdmv_pgs_subtitle' }), track(2, 'jpn')];
  assert.deepEqual(S.textSubtitleTracks(tracks).map((t) => t.index), [1, 2]);
  assert.deepEqual(S.textSubtitleTracks(undefined), []);
  assert.equal(S.isBitmapSubtitle(tracks[1]), true);
  assert.equal(S.isBitmapSubtitle(tracks[2]), false);
  assert.equal(S.isBitmapSubtitle(undefined), false);
});

test('pickDefaultSubtitle prefers a full French track over forced or signs tracks', () => {
  const forced = track(0, 'fre', 'Forced', { is_forced: true });
  const signs = track(1, 'fre', 'Signs & Songs');
  const english = track(2, 'eng', 'English', { is_default: true });
  const french = track(3, 'fra', 'Français');
  assert.equal(S.pickDefaultSubtitle([forced, signs, english, french]).index, 3);
  assert.equal(S.pickDefaultSubtitle([forced, signs, english]).index, 2, 'no full French track: the default one');
  assert.equal(S.pickDefaultSubtitle([forced, signs]).index, 0, 'only partial tracks: French among them');
  assert.equal(S.pickDefaultSubtitle([track(4, 'und', 'VOSTFR')]).index, 4);
});

test('libraryLoad and initialFileIndex: library copies are file 0, torrents wait for matching', () => {
  const torrent = { id: 'a', info_hash: 'a', magnet_uri: 'm', title: 't', size_bytes: 1, seeders: 1 };
  const library = { ...torrent, library: { stream_id: 'lib-1' } };
  assert.equal(S.libraryLoad(null), null);
  assert.equal(S.libraryLoad(torrent), null);
  assert.equal(S.libraryLoad(library).info_hash, 'lib-1');
  assert.equal(S.libraryLoad(library).files.length, 1);
  assert.equal(S.initialFileIndex(null), 0);
  assert.equal(S.initialFileIndex(torrent), -1);
  assert.equal(S.initialFileIndex(library), 0);
});

test('isLibrarySource: only items carrying a library copy', () => {
  const torrent = { id: 'a', info_hash: 'a', magnet_uri: 'm', title: 't', size_bytes: 1, seeders: 1 };
  assert.equal(S.isLibrarySource(null), false);
  assert.equal(S.isLibrarySource(torrent), false);
  assert.equal(S.isLibrarySource({ ...torrent, library: undefined }), false);
  assert.equal(S.isLibrarySource({ ...torrent, library: { stream_id: 'lib-1' } }), true);
});

test('matchedFileIndex: episodes match by name, other items take the main video', () => {
  const files = [{ index: 0, path: 'Show - 01.mkv', length: 1, is_video: true }, { index: 1, path: 'Show - 02.mkv', length: 1, is_video: true }];
  const data = { info_hash: 'h', files, main_video_index: 0 };
  const torrent = { id: 'a', info_hash: 'a', magnet_uri: 'm', title: 'Show', size_bytes: 1, seeders: 1 };
  assert.equal(S.matchedFileIndex(data, torrent), 0, 'not an episode: main video');
  assert.equal(S.matchedFileIndex(data, { ...torrent, episode_number: 0 }), 0, 'episode 0 is not an episode');
  assert.equal(S.matchedFileIndex(data, { ...torrent, episode_number: 2, season_number: 1, anime_aliases: ['Show'] }), 1);
  assert.equal(S.matchedFileIndex(data, { ...torrent, episode_number: 7, season_number: 1, anime_aliases: ['Show'] }), null, 'missing episode: ask');
});

test('progressTracks reports track languages, "" when subtitles are off', () => {
  const meta = { duration_sec: 1, audio_tracks: [{ index: 1, language: 'jpn' }, { index: 2, language: 'fre' }], subtitle_tracks: [track(3, 'fre')] };
  assert.deepEqual(S.progressTracks(meta, 2, 3), { audioLang: 'fre', subLang: 'fre' });
  assert.deepEqual(S.progressTracks(meta, 2, null), { audioLang: 'fre', subLang: '' });
  assert.deepEqual(S.progressTracks(meta, 9, 9), { audioLang: undefined, subLang: undefined });
  assert.deepEqual(S.progressTracks(null, 0, null), { audioLang: undefined, subLang: '' });
});

test('clampSeek keeps targets inside the known duration', () => {
  assert.equal(S.clampSeek(-3, 100), 0);
  assert.equal(S.clampSeek(150, 100), 100);
  assert.equal(S.clampSeek(150, 0), 150, 'unknown duration: no upper bound');
  assert.equal(S.clampSeek(42, 100), 42);
});

test('stepVolume starts from zero when muted and snaps to 5 % steps', () => {
  assert.equal(S.stepVolume(0.5, false, true), 0.55);
  assert.equal(S.stepVolume(0.5, true, true), 0.05);
  assert.equal(S.stepVolume(1, false, true), 1);
  assert.equal(S.stepVolume(0.02, false, false), 0);
  assert.equal(S.stepVolume(0.33, false, false), 0.3);
});

test('progressPercent and bufferedSpan stay within the bar', () => {
  assert.equal(S.progressPercent(50, 200), 25);
  assert.equal(S.progressPercent(300, 200), 100);
  assert.equal(S.progressPercent(-1, 200), 0);
  // Remux started at 100 s: the bar starts there and ends at the range holding the playhead.
  assert.deepEqual(S.bufferedSpan([[0, 10], [30, 60]], 5, 100, 1000), { left: 10, width: 1 });
  assert.deepEqual(S.bufferedSpan([[0, 10], [10.2, 60]], 9.8, 100, 1000), { left: 10, width: 6 });
  assert.deepEqual(S.bufferedSpan([], 5, 0, 100), { left: 0, width: 5 });
});

test('scrubberTarget maps slider keys and clamps to the timeline', () => {
  assert.equal(S.scrubberTarget('ArrowRight', 10, 100), 15);
  assert.equal(S.scrubberTarget('ArrowDown', 3, 100), 0);
  assert.equal(S.scrubberTarget('PageUp', 90, 100), 100);
  assert.equal(S.scrubberTarget('Home', 50, 100), 0);
  assert.equal(S.scrubberTarget('End', 50, 100), 100);
  assert.equal(S.scrubberTarget('Enter', 50, 100), null);
});

test('playerShortcut maps keys to actions and leaves modified or unavailable ones alone', () => {
  const all = { hasNextEpisode: true, canPip: true, hasSkip: true };
  const none = { hasNextEpisode: false, canPip: false, hasSkip: false };
  assert.deepEqual(S.playerShortcut({ code: 'Space' }, none), { action: 'toggle-play' });
  assert.deepEqual(S.playerShortcut({ code: 'KeyK', ctrlKey: true }, none), { action: 'toggle-play' });
  assert.deepEqual(S.playerShortcut({ code: 'KeyJ' }, none), { action: 'seek', delta: -10 });
  assert.deepEqual(S.playerShortcut({ code: 'ArrowRight' }, none), { action: 'seek', delta: 10 });
  assert.deepEqual(S.playerShortcut({ code: 'ArrowUp' }, none), { action: 'volume', up: true });
  assert.deepEqual(S.playerShortcut({ code: 'KeyF' }, none), { action: 'fullscreen' });
  assert.deepEqual(S.playerShortcut({ code: 'KeyM' }, none), { action: 'mute' });
  assert.deepEqual(S.playerShortcut({ code: 'KeyN' }, all), { action: 'next-episode' });
  assert.equal(S.playerShortcut({ code: 'KeyN' }, none), null);
  assert.equal(S.playerShortcut({ code: 'KeyN', metaKey: true }, all), null);
  assert.deepEqual(S.playerShortcut({ code: 'KeyP' }, all), { action: 'pip' });
  assert.equal(S.playerShortcut({ code: 'KeyP' }, none), null);
  assert.deepEqual(S.playerShortcut({ code: 'Period', shiftKey: true }, none), { action: 'rate', direction: 1 });
  assert.deepEqual(S.playerShortcut({ code: 'Comma', shiftKey: true }, none), { action: 'rate', direction: -1 });
  assert.equal(S.playerShortcut({ code: 'Period' }, none), null, 'a plain period is not a rate change');
  assert.deepEqual(S.playerShortcut({ code: 'KeyS' }, all), { action: 'skip' });
  assert.equal(S.playerShortcut({ code: 'KeyS', altKey: true }, all), null);
  assert.equal(S.playerShortcut({ code: 'KeyZ' }, all), null);
});

test('episodeFileKey needs a season, an episode and a resolved file', () => {
  assert.equal(S.episodeFileKey(7, 3, 'abc', 2), '7:3:abc:2');
  assert.equal(S.episodeFileKey(0, 3, 'abc', 2), '');
  assert.equal(S.episodeFileKey(NaN, 3, 'abc', 2), '');
  assert.equal(S.episodeFileKey(7, undefined, 'abc', 2), '');
  assert.equal(S.episodeFileKey(7, 3, undefined, 2), '');
  assert.equal(S.episodeFileKey(7, 3, 'abc', -1), '');
});

test('fileSelectionFailure tells a missing episode from an ambiguous one', () => {
  assert.match(S.fileSelectionFailure(0).reason, /introuvable/);
  assert.match(S.fileSelectionFailure(2).reason, /ambiguïté/);
  assert.equal(S.fileSelectionFailure(2).code, 'episode_missing_or_ambiguous');
});

test('metadataTimedOut: idle swarm or hard cap', () => {
  assert.equal(S.metadataTimedOut(29, 0, 0, timeouts), false);
  assert.equal(S.metadataTimedOut(30, 0, 0, timeouts), true);
  assert.equal(S.metadataTimedOut(99, 0, 90, timeouts), false);
  assert.equal(S.metadataTimedOut(100, 0, 99, timeouts), true, 'activity never extends past max');
});

test('streamWatchdog: startup timeout before the first frame, stall only when both playback and swarm stop', () => {
  const base = { startedAt: 0, lastActivity: 0, lastProgressAt: 0, hasStarted: false };
  assert.equal(S.streamWatchdog({ ...base, now: 19 }, timeouts), null);
  assert.equal(S.streamWatchdog({ ...base, now: 20 }, timeouts).code, 'startup_timeout');
  assert.equal(S.streamWatchdog({ ...base, now: 100, lastActivity: 99 }, timeouts).code, 'startup_timeout');
  const playing = { ...base, hasStarted: true };
  assert.equal(S.streamWatchdog({ ...playing, now: 50, lastProgressAt: 40, lastActivity: 0 }, timeouts), null, 'still playing');
  assert.equal(S.streamWatchdog({ ...playing, now: 50, lastProgressAt: 0, lastActivity: 40 }, timeouts), null, 'still downloading');
  assert.equal(S.streamWatchdog({ ...playing, now: 50, lastProgressAt: 30, lastActivity: 35 }, timeouts).code, 'swarm_stall');
  assert.equal(S.streamWatchdog({ ...playing, now: 500, lastProgressAt: 490, lastActivity: 0 }, timeouts), null, 'no hard cap once started');
});

test('mediaErrorMessage names HEVC only for decode errors on HEVC', () => {
  assert.match(S.mediaErrorMessage(3, 'HEVC'), /H\.265/);
  assert.match(S.mediaErrorMessage(4, 'h265'), /H\.265/);
  assert.match(S.mediaErrorMessage(4, 'h264'), /Impossible de décoder/);
  assert.match(S.mediaErrorMessage(2, 'hevc'), /interrompue/);
  assert.match(S.mediaErrorMessage(undefined), /interrompue/);
});

test('endedEarly: only with a known duration and more than 2 s left', () => {
  assert.equal(S.endedEarly(50, 100), true);
  assert.equal(S.endedEarly(98.5, 100), false);
  assert.equal(S.endedEarly(50, 0), false);
});
