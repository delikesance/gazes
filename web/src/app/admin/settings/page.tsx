import { SettingsView } from "@/components/admin/pages/SettingsView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminEnvelope, AdminSettings, AdminTokens } from "@/lib/admin/types";

export default async function Page() {
  let settings: AdminEnvelope<AdminSettings>;
  try {
    settings = await adminGetServer<AdminSettings>("/settings");
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Accès et alertes" title="Paramètres" periods={false} />
        <ApiErrorBlock error={error} />
      </>
    );
  }
  // The token list is for a human session only: show the rest of the page if it cannot be read.
  let tokens: AdminTokens | null = null;
  try {
    tokens = (await adminGetServer<AdminTokens>("/tokens")).data;
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
  }
  return <SettingsView settings={settings.data} tokens={tokens} generatedAt={settings.generated_at} />;
}
