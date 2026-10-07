import { NextResponse, type NextRequest } from "next/server";
import { adminGet } from "@/lib/admin/api";
import { gateFailureCode, isAdminRefusal } from "@/lib/admin/gate";

/**
 * Admin gate before any rendering. Next renders a layout and its page in parallel, so the check in
 * app/admin/layout.tsx alone still let the page's output (titles, section names) stream into the 404 sent to
 * a stranger. Anyone the API does not confirm as an admin, outages included, gets the site's generic 404.
 */
export async function proxy(request: NextRequest) {
  try {
    await adminGet("/me", { cookie: request.headers.get("cookie") ?? "" });
    return NextResponse.next();
  } catch (error) {
    if (!isAdminRefusal(error)) console.error(`admin gate unavailable: ${gateFailureCode(error)}`);
    return NextResponse.rewrite(new URL("/_introuvable", request.url));
  }
}

export const config = { matcher: ["/admin", "/admin/:path*"] };
