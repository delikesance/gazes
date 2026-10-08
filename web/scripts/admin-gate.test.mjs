// Admin gate: which failures are refusals, and what reaches the server logs.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { register } from 'node:module';

// gate.ts imports ./api without extension (bundler resolution): teach node to find the .ts and the @/ alias.
register('data:text/javascript,' + encodeURIComponent(`
export async function resolve(specifier, context, next) {
  if (specifier.startsWith('.') && !/\\.[a-z]+$/.test(specifier)) {
    try { return await next(specifier + '.ts', context); } catch {}
  }
  // @/lib/api pulls the whole site client (non-erasable TS); the admin client only needs getApiBase from it.
  if (specifier === '@/lib/api') return { url: 'data:text/javascript,export const getApiBase = () => "http://api.test/api/v1";', shortCircuit: true };
  if (specifier.startsWith('@/')) return next(new URL('../src/' + specifier.slice(2) + '.ts', ${JSON.stringify(import.meta.url)}).href, context);
  return next(specifier, context);
}`));

const { AdminApiError } = await import('../src/lib/admin/api.ts');
const { isAdminRefusal, gateFailureCode } = await import('../src/lib/admin/gate.ts');
const { translate } = await import('../src/lib/i18n.ts');

test('signed out, not an admin and admin API not mounted are refusals', () => {
  for (const status of [401, 403, 404]) assert.equal(isAdminRefusal(new AdminApiError(status, 'x', 'x')), true, String(status));
});

test('an outage is not a refusal (it is logged, still answered with a 404)', () => {
  assert.equal(isAdminRefusal(new AdminApiError(0, 'network', 'Le serveur est injoignable')), false);
  assert.equal(isAdminRefusal(new AdminApiError(502, 'http_error', 'Bad Gateway')), false);
  assert.equal(isAdminRefusal(new Error('boom')), false);
  assert.equal(isAdminRefusal(undefined), false);
});

test('logs carry the status and code only, never the message', () => {
  assert.equal(gateFailureCode(new AdminApiError(0, 'network', 'secret detail')), '0/network');
  assert.equal(gateFailureCode(new AdminApiError(500, 'internal', 'db password wrong')), '500/internal');
  assert.equal(gateFailureCode(new Error('stack here')), 'unexpected');
});

test('the 404 page is translated', () => {
  assert.equal(translate('fr', 'Page introuvable'), 'Page introuvable');
  assert.equal(translate('en', 'Page introuvable'), 'Page not found');
  assert.equal(translate('en', 'Cette page n’existe pas ou n’est plus disponible.'), 'This page does not exist or is no longer available.');
});
