import { test } from 'node:test';
import assert from 'node:assert/strict';
import { isBareChromePath } from '../src/components/SiteChrome.logic.ts';

test('admin routes hide the public chrome', () => {
  for (const p of ['/admin', '/admin/', '/admin/views', '/admin/playback', '/admin-preview', '/admin-preview/overview']) {
    assert.equal(isBareChromePath(p), true, p);
  }
});

test('public routes keep the public chrome', () => {
  for (const p of ['/', '/anime/12', '/administrator', '/administration/x', '/search', '/privacy', '', null, undefined]) {
    assert.equal(isBareChromePath(p), false, String(p));
  }
});
