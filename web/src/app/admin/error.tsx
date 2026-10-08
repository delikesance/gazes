"use client";
import { useRouter } from "next/navigation";
import { useTransition } from "react";

/**
 * Error boundary of the admin pages. It renders inside the admin frame, so the layout has already checked that
 * the visitor is an admin: offering a retry reveals nothing. The error itself is never shown (message, digest
 * and stack stay in the server logs).
 */
export default function AdminError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  // A server component failed: refresh its payload, then re-render the segment.
  const retry = () => startTransition(() => { router.refresh(); reset(); });
  return (
    <section role="alert" className="admin-empty admin-error">
      <h2>Cette page n&apos;a pas pu s&apos;afficher</h2>
      <p>Le serveur ne répond pas ou a renvoyé une erreur. Réessayez dans un instant.</p>
      <button type="button" className="admin-retry" onClick={retry} disabled={pending}>
        {pending ? "Nouvel essai…" : "Réessayer"}
      </button>
    </section>
  );
}
