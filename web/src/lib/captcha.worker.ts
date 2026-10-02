/// <reference lib="webworker" />
import { sha256 } from "@noble/hashes/sha2.js";

interface Job { challenge: string; salt: string; maxnumber: number }

const encoder = new TextEncoder();
const hex = (bytes: Uint8Array) => Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");

// ALTCHA proof-of-work: find n such that sha256(salt + n) === challenge.
self.onmessage = (event: MessageEvent<Job>) => {
  const { challenge, salt, maxnumber } = event.data;
  for (let n = 0; n <= maxnumber; n++) {
    if (hex(sha256(encoder.encode(salt + n))) === challenge) {
      (self as DedicatedWorkerGlobalScope).postMessage({ number: n });
      return;
    }
  }
  (self as DedicatedWorkerGlobalScope).postMessage({ number: -1 });
};
