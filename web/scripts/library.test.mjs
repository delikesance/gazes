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
