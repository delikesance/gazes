// Interop test: the browser-side crypto (src/lib/auth-crypto.ts, noble) against the
// real Go backend. Needs a running stack: BASE_URL defaults to http://localhost:8080.
import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { build } from "esbuild";
import { sha256 } from "@noble/hashes/sha2.js";

const root = path.resolve(import.meta.dirname, "..");
const base = (process.env.BASE_URL || "http://localhost:8080") + "/api/v1";
const tmp = await mkdtemp(path.join(tmpdir(), "auth-envelope-"));
await build({ absWorkingDir: root, entryPoints: ["src/lib/auth-crypto.ts"], outfile: path.join(tmp, "auth-crypto.mjs"), bundle: true, format: "esm", platform: "node", logLevel: "silent" });
const { sealEnvelope, toBase64 } = await import(path.join(tmp, "auth-crypto.mjs"));

const enc = new TextEncoder();
const hex = (bytes) => Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
const get = (p, headers = {}) => fetch(base + p, { headers });
const post = (p, body, headers = {}) => fetch(base + p, { method: "POST", headers: { "Content-Type": "application/json", ...headers }, body: JSON.stringify(body) });

async function captcha() {
  const c = await (await get("/auth/captcha")).json();
  for (let n = 0; n <= c.maxnumber; n++) {
    if (hex(sha256(enc.encode(c.salt + n))) === c.challenge)
      return toBase64(enc.encode(JSON.stringify({ algorithm: c.algorithm, challenge: c.challenge, number: n, salt: c.salt, signature: c.signature })));
  }
  throw new Error("captcha unsolvable");
}
const kem = async () => (await get("/auth/kem")).json();
async function sealed(route, payload, info) {
  return sealEnvelope(info || (await kem()), route, payload);
}

try {
  const stamp = Date.now().toString(36);
  const email = `interop-${stamp}@example.com`, password = "correct horse battery", pseudo = `interop${stamp}`.slice(0, 24);

  // Register, then confirm the session cookie and /auth/me.
  const reg = await post("/auth/register", await sealed("register", { email, password, pseudo, captcha: await captcha() }));
  assert.equal(reg.status, 201, await reg.clone().text());
  const cookie = reg.headers.getSetCookie().find((c) => c.startsWith("gazes_session="));
  assert.ok(cookie && /HttpOnly/i.test(cookie) && /SameSite=Lax/i.test(cookie), "session cookie flags");
  const me = await (await get("/auth/me", { Cookie: cookie.split(";")[0] })).json();
  assert.equal(me.user.email, email);

  // Login works with the same credentials; a wrong password is a generic 401.
  const ok = await post("/auth/login", await sealed("login", { email, password, captcha: await captcha() }));
  assert.equal(ok.status, 200, await ok.clone().text());
  const bad = await post("/auth/login", await sealed("login", { email, password: "wrong password!", captcha: await captcha() }));
  assert.equal(bad.status, 401);
  assert.equal((await bad.json()).error, "invalid_credentials");

  // A replayed envelope is rejected (its nonce is single-use).
  const info = await kem();
  const env = await sealed("login", { email, password, captcha: await captcha() }, info);
  assert.equal((await post("/auth/login", env)).status, 200);
  assert.equal((await post("/auth/login", env)).status, 400, "replay must fail");

  // A tampered envelope is rejected.
  const tampered = await sealed("login", { email, password, captcha: await captcha() });
  const raw = Buffer.from(tampered.data, "base64");
  raw[0] ^= 1;
  tampered.data = raw.toString("base64");
  assert.equal((await post("/auth/login", tampered)).status, 400, "tampering must fail");

  // An envelope sealed for one route does not open on another.
  const wrongRoute = await sealed("register", { email: `x-${stamp}@example.com`, password, pseudo: "wrongroute", captcha: await captcha() });
  assert.equal((await post("/auth/login", wrongRoute)).status, 400, "route binding");

  // Registration and login require a valid captcha.
  const noCaptcha = await post("/auth/login", await sealed("login", { email, password, captcha: "" }));
  assert.equal((await noCaptcha.json()).error, "captcha_failed");

  // Cross-site posts are refused.
  const forged = await post("/auth/login", await sealed("login", { email, password, captcha: await captcha() }), { Origin: "http://evil.test" });
  assert.equal(forged.status, 403);

  console.log("auth envelope interop: all checks passed");
} finally {
  await rm(tmp, { recursive: true, force: true });
}
