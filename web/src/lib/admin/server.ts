import { cookies } from "next/headers";
import { adminGet, type AdminEnvelope, type AdminGetOptions } from "./api";

/** Forwards the visitor's cookies; use from server components (they are not sent automatically). */
export async function adminGetServer<T>(path: string, options: Omit<AdminGetOptions, "cookie"> = {}): Promise<AdminEnvelope<T>> {
  return adminGet<T>(path, { ...options, cookie: (await cookies()).toString() });
}
