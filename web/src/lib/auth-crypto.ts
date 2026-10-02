import { gcm } from "@noble/ciphers/aes.js";
import { hkdf } from "@noble/hashes/hkdf.js";
import { sha256 } from "@noble/hashes/sha2.js";
import { ml_kem768 } from "@noble/post-quantum/ml-kem.js";

/** Server answer of GET /auth/kem. */
export interface KemInfo { kid: string; ek: string; nonce: string }

/** Encrypted request body; must match internal/auth/envelope.go. */
export interface Envelope { kid: string; ct: string; nonce: string; iv: string; data: string }

const INFO_PREFIX = "gazes-auth-v1|";
const encoder = new TextEncoder();

export function toBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function fromBase64(value: string): Uint8Array {
  const binary = atob(value);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0));
  let offset = 0;
  for (const part of parts) { out.set(part, offset); offset += part.length; }
  return out;
}

/**
 * Wrap `payload` for `route` in a post-quantum envelope:
 * ML-KEM-768 encapsulation to the server key, HKDF-SHA256 (salt = server nonce),
 * AES-256-GCM with the key id, route and nonce as associated data.
 * Pure JS (noble), so it also works on plain-HTTP origins where WebCrypto is absent.
 */
export function sealEnvelope(info: KemInfo, route: string, payload: unknown): Envelope {
  const nonce = fromBase64(info.nonce);
  const { cipherText, sharedSecret } = ml_kem768.encapsulate(fromBase64(info.ek));
  const key = hkdf(sha256, sharedSecret, nonce, encoder.encode(INFO_PREFIX + route), 32);
  const iv = crypto.getRandomValues(new Uint8Array(12));
  const aad = concat(encoder.encode(`${info.kid}|${route}|`), nonce);
  const data = gcm(key, iv, aad).encrypt(encoder.encode(JSON.stringify(payload)));
  return { kid: info.kid, ct: toBase64(cipherText), nonce: info.nonce, iv: toBase64(iv), data: toBase64(data) };
}
