/** Error that carries a short, greppable code (e.g. `CAL_HTTP_502`) and an optional diagnostic reference. */
export class CodedError extends Error {
  constructor(message: string, public code: string, public diagnosticReference?: string | null) {
    super(message);
  }
}

/** `AREA_HTTP_<status>` for a failed response. */
export function httpError(message: string, area: string, response: Response, reference?: string | null): CodedError {
  return new CodedError(message, `${area}_HTTP_${response.status}`, reference ?? response.headers.get("X-Playback-Session-ID") ?? response.headers.get("X-Request-ID"));
}

/** Code for any thrown value: its own code, `AREA_NETWORK` for fetch failures, else `AREA_UNKNOWN`. */
export function errorCode(error: unknown, area: string): string {
  if (error instanceof CodedError) return error.code;
  if (error instanceof TypeError) return `${area}_NETWORK`;
  return `${area}_UNKNOWN`;
}
