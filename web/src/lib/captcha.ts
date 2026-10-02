import { toBase64 } from "./auth-crypto";

/** Server answer of GET /auth/captcha (ALTCHA format). */
export interface CaptchaChallenge { algorithm: string; challenge: string; salt: string; signature: string; maxnumber: number }

/** Solve a challenge off the main thread and return the base64 payload the server verifies. */
export function solveCaptcha(challenge: CaptchaChallenge, signal?: AbortSignal): Promise<string> {
  return new Promise((resolve, reject) => {
    const worker = new Worker(new URL("./captcha.worker.ts", import.meta.url));
    const done = () => worker.terminate();
    signal?.addEventListener("abort", () => { done(); reject(new DOMException("aborted", "AbortError")); }, { once: true });
    worker.onerror = () => { done(); reject(new Error("captcha_failed")); };
    worker.onmessage = (event: MessageEvent<{ number: number }>) => {
      done();
      if (event.data.number < 0) return reject(new Error("captcha_failed"));
      const payload = { algorithm: challenge.algorithm, challenge: challenge.challenge, number: event.data.number, salt: challenge.salt, signature: challenge.signature };
      resolve(toBase64(new TextEncoder().encode(JSON.stringify(payload))));
    };
    worker.postMessage({ challenge: challenge.challenge, salt: challenge.salt, maxnumber: challenge.maxnumber });
  });
}
