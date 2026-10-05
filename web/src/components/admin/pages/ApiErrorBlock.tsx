import { AdminApiError } from "@/lib/admin/api";

/** Readable error block for a failed admin call: message + code, never a stack trace. */
export function ApiErrorBlock({ error, title = "Les mesures n'ont pas pu être chargées" }: { error: unknown; title?: string }) {
  const code = error instanceof AdminApiError ? error.code : "unknown";
  const message = error instanceof AdminApiError ? error.message : "Erreur inattendue";
  return (
    <section role="alert" className="admin-empty" style={{ alignItems: "flex-start", textAlign: "left" }}>
      <h2>{title}</h2>
      <p>{message}</p>
      <p style={{ fontFamily: "var(--font-geist-mono), monospace" }}>Code : {code}</p>
    </section>
  );
}
