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

test('the chrome stays until the admin frame is mounted (a stranger’s 404 at /admin keeps the header)', async () => {
  const { hidesChrome } = await import('../src/components/SiteChrome.logic.ts');
  assert.equal(hidesChrome('/admin', false), false);
  assert.equal(hidesChrome('/admin/views', true), true);
  assert.equal(hidesChrome('/admin-preview', true), true);
  assert.equal(hidesChrome('/anime/12', true), false);
});
