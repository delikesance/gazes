import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { SettingsView } from "@/components/admin/pages/SettingsView";
import type { AdminEnvelope, AdminSettings, AdminTokens } from "@/lib/admin/types";
import settingsJson from "@/lib/admin/real/settings.json";
import tokensJson from "@/lib/admin/real/tokens.json";
import "@/app/admin/admin.css";

const settings = settingsJson as AdminEnvelope<AdminSettings>;
const tokens = tokensJson as AdminEnvelope<AdminTokens>;

/** Dev-only preview of "Paramètres" with real recorded responses (no network, no guard, no writes). */
export default function Page() {
  if (process.env.NODE_ENV === "production") notFound();
  return (
    <AdminShell>
      <SettingsView settings={settings.data} tokens={tokens.data} generatedAt={settings.generated_at} demo />
    </AdminShell>
  );
}
