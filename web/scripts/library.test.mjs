import { test } from 'node:test';
import assert from 'node:assert/strict';
import { libraryLang, canPlayCopy, librarySource, withLibraryCandidates, AV1_MIME } from '../src/lib/library.ts';

const src = (hash, tag) => ({ info_hash: hash, id: hash, title: hash, magnet_uri: `magnet:?xt=urn:btih:${hash}`, language_tag: tag, is_french: tag === 'VF' || tag === 'MULTI', seeders: 5 });
const copy = (lang, codec = 'av1', state = codec === 'av1' ? 'AV1' : 'ORIGINAL') => ({ lang, state, video_codec: codec, stream_id: `lib-${lang}-${codec}`, duration_ms: 1400000, audio_tracks: 2, subtitle_tracks: 1 });
const yes = () => true;
const no = () => false;

test('libraryLang maps VF and MULTI to vf, VOSTFR to vostfr, others to null', () => {
  assert.equal(libraryLang(src('a', 'VF')), 'vf');
  assert.equal(libraryLang(src('a', 'MULTI')), 'vf');
  assert.equal(libraryLang(src('a', 'VOSTFR')), 'vostfr');
  for (const tag of ['VOSTEN', 'RAW', 'OTHER']) assert.equal(libraryLang(src('a', tag)), null);
});

test('canPlayCopy rejects av1 when unsupported, accepts original copies', () => {
  const seen = [];
  assert.equal(canPlayCopy(copy('vf'), (m) => { seen.push(m); return false; }), false);
  assert.deepEqual(seen, [AV1_MIME]);
  assert.equal(canPlayCopy(copy('vf'), yes), true);
  assert.equal(canPlayCopy(copy('vf', 'h264'), no), true);
});

test('withLibraryCandidates puts a vf copy before the first VF torrent source', () => {
  const ranked = [src('t1', 'VOSTFR'), src('t2', 'VF'), src('t3', 'VF')];
  const out = withLibraryCandidates(ranked, [copy('vf')], yes);
  assert.deepEqual(out.map((s) => s.info_hash), ['t1', 'lib-vf-av1', 't2', 't3']);
});

test('withLibraryCandidates keeps a VF torrent ahead of a cached vostfr copy', () => {
  const ranked = [src('t1', 'VF'), src('t2', 'VOSTFR')];
  const out = withLibraryCandidates(ranked, [copy('vostfr')], yes);
  assert.deepEqual(out.map((s) => s.info_hash), ['t1', 'lib-vostfr-av1', 't2']);
});

test('withLibraryCandidates appends a copy when no source has its language, ignoring non-library-able ones', () => {
  const out = withLibraryCandidates([src('t1', 'VOSTEN')], [copy('vf')], yes);
  assert.deepEqual(out.map((s) => s.info_hash), ['t1', 'lib-vf-av1']);
});

test('withLibraryCandidates drops unplayable av1 copies', () => {
  const ranked = [src('t1', 'VF')];
  assert.deepEqual(withLibraryCandidates(ranked, [copy('vf')], no).map((s) => s.info_hash), ['t1']);
  assert.deepEqual(withLibraryCandidates(ranked, [copy('vf', 'h264')], no).map((s) => s.info_hash), ['lib-vf-h264', 't1']);
});

test('librarySource uses stream_id as info_hash and has no magnet', () => {
  const s = librarySource(copy('vostfr'), { title: 'Show', episode_number: 3 });
  assert.equal(s.info_hash, 'lib-vostfr-av1');
  assert.equal(s.magnet_uri, '');
  assert.equal(s.language_tag, 'VOSTFR');
  assert.equal(s.is_french, true);
  assert.equal(s.episode_number, 3);
  assert.deepEqual(s.library, { stream_id: 'lib-vostfr-av1', lang: 'vostfr', video_codec: 'av1', duration_ms: 1400000 });
  assert.ok(s.seeders > 1000);
});

import { lookupWithin } from '../src/lib/library.ts';

test('lookupWithin resolves empty on timeout, on rejection and on a throwing lookup, and returns copies otherwise', async () => {
  const c = copy('vf');
  assert.deepEqual((await lookupWithin(50, async () => [c])).copies, [c]);
  assert.deepEqual(await lookupWithin(20, () => new Promise(() => {})), { copies: [], timedOut: true });
  assert.deepEqual(await lookupWithin(50, async () => { throw new Error('x'); }), { copies: [], timedOut: false });
  assert.deepEqual(await lookupWithin(50, () => { throw new Error('x'); }), { copies: [], timedOut: false });
});

import { loadWatchData, prewarmTargets, playerWaitState, currentFor } from '../src/lib/library.ts';

const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => { resolve = a; reject = b; }); return { promise, resolve, reject }; };
const tick = () => new Promise((r) => setTimeout(r, 5));

function watch(parts) {
  const events = [];
  const done = loadWatchData(parts, {
    ready: (d) => events.push(['ready', d]),
    sources: (s) => events.push(['sources', s]),
    sourcesFailed: (e) => events.push(['sourcesFailed', e]),
  });
  return { events, done };
}

test('loadWatchData starts playback from a library copy without waiting for the sources', async () => {
  const sources = deferred();
  const c = copy('vf');
  const { events, done } = watch({ franchise: Promise.resolve('F'), season: Promise.resolve('S'), sources: sources.promise, copies: Promise.resolve([c]) });
  await tick();
  assert.equal(events.length, 1);
  assert.deepEqual(events[0], ['ready', { franchise: 'F', season: 'S', copies: [c], sources: undefined, sourcesState: 'pending' }]);
  sources.resolve('SRC');
  await done;
  assert.deepEqual(events[1], ['sources', 'SRC']);
});

test('loadWatchData waits for the sources when there is no copy', async () => {
  const sources = deferred();
  const { events, done } = watch({ franchise: Promise.resolve('F'), season: Promise.resolve('S'), sources: sources.promise, copies: Promise.resolve([]) });
  await tick();
  assert.equal(events.length, 0);
  sources.resolve('SRC');
  await done;
  assert.deepEqual(events, [['ready', { franchise: 'F', season: 'S', copies: [], sources: 'SRC', sourcesState: 'loaded' }]]);
});

test('loadWatchData reports a sources failure only when no copy can play', async () => {
  const failing = () => { const d = deferred(); d.reject(new Error('sources down')); return d.promise; };
  await assert.rejects(loadWatchData({ franchise: Promise.resolve('F'), season: Promise.resolve('S'), sources: failing(), copies: Promise.resolve([]) }, { ready() {}, sources() {} }), /sources down/);
  const { events, done } = watch({ franchise: Promise.resolve('F'), season: Promise.resolve('S'), sources: failing(), copies: Promise.resolve([copy('vf')]) });
  await done;
  assert.deepEqual(events.map((e) => e[0]), ['ready']);
  assert.equal(events[0][1].sources, undefined);
  assert.equal(events[0][1].sourcesState, 'failed');
});

test('loadWatchData reports a late sources failure with a copy as failed, not pending', async () => {
  const sources = deferred();
  const { events, done } = watch({ franchise: Promise.resolve('F'), season: Promise.resolve('S'), sources: sources.promise, copies: Promise.resolve([copy('vf')]) });
  await tick();
  assert.equal(events[0][1].sourcesState, 'pending');
  const err = new Error('late');
  sources.reject(err);
  await done;
  assert.deepEqual(events[1], ['sourcesFailed', err]);
  assert.equal(events.length, 2);
});

test('playerWaitState shows pending only while the sources request is in flight', () => {
  assert.equal(playerWaitState({ hasSource: false, sourcesState: 'pending' }), 'pending');
  assert.equal(playerWaitState({ hasSource: false, sourcesState: 'failed' }), 'exhausted');
  assert.equal(playerWaitState({ hasSource: false, sourcesState: 'loaded' }), 'exhausted');
  assert.equal(playerWaitState({ hasSource: true, sourcesState: 'pending' }), 'playing');
});

test('loadWatchData passes already available sources with the copies in one step', async () => {
  const { events, done } = watch({ franchise: Promise.resolve('F'), season: Promise.resolve('S'), sources: Promise.resolve('SRC'), copies: Promise.resolve([copy('vf')]) });
  await done;
  assert.deepEqual(events, [['ready', { franchise: 'F', season: 'S', copies: [copy('vf')], sources: 'SRC', sourcesState: 'loaded' }]]);
});

test('loadWatchData still fails when the catalog fails', async () => {
  const failing = Promise.reject(new Error('catalog'));
  await assert.rejects(loadWatchData({ franchise: failing, season: Promise.resolve('S'), sources: Promise.resolve('x'), copies: Promise.resolve([]) }, { ready() {}, sources() {} }), /catalog/);
});

test('prewarmTargets skips torrent prewarming while a library copy plays', () => {
  const lib = librarySource(copy('vf'), { title: 'T', episode_number: 1 });
  const list = [lib, src('t1', 'VF'), src('t2', 'VF'), src('t3', 'VF')];
  assert.deepEqual(prewarmTargets(list, 0), []);
  assert.deepEqual(prewarmTargets(list, 1).map((s) => s.info_hash), ['t2', 't3']);
  assert.deepEqual(prewarmTargets([src('t1', 'VF'), lib, src('t2', 'VF')], 0).map((s) => s.info_hash), ['t2']);
  assert.deepEqual(prewarmTargets([], 0), []);
});

test('currentFor only exposes data loaded for the current request key', () => {
  const loaded = { key: '1/2/3:0', sourcesState: 'failed' };
  assert.equal(currentFor(loaded, '1/2/3:0'), loaded);
  assert.equal(currentFor(loaded, '1/2/3:1'), null);
  assert.equal(currentFor(loaded, '1/2/4:0'), null);
  assert.equal(currentFor(null, '1/2/3:0'), null);
});
