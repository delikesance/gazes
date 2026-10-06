import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
const read = (p) => readFileSync(new URL(`../../${p}`, import.meta.url), 'utf8');
const compose = read('compose.yaml');
const dockerfile = read('deploy/web.Dockerfile');
// The backend shares gluetun's network namespace: the only names that reach it are gluetun and its alias.
test('the web image bakes the same backend address compose gives it at runtime', () => {
 const baked = dockerfile.match(/^ARG BACKEND_URL=(\S+)/m)?.[1];
 const runtime = compose.match(/^\s+BACKEND_URL: (\S+)/m)?.[1];
 const buildArg = compose.match(/^\s+args:\s*\n\s+BACKEND_URL: (\S+)/m)?.[1];
 assert.ok(baked && runtime && buildArg, 'BACKEND_URL must be set in the Dockerfile, the web environment and build.args');
 assert.equal(baked, runtime);
 assert.equal(buildArg, runtime);
 assert.match(new URL(runtime).hostname, /^(gluetun|backend)$/);
});
test('the Dockerfile never hardcodes an unreachable backend host', () => {
 assert.doesNotMatch(dockerfile, /^ENV .*BACKEND_URL=http:\/\/backend/m);
});
test('gluetun keeps the backend alias so older callers still resolve', () => {
 assert.match(compose, /aliases: \[backend\]/);
});
