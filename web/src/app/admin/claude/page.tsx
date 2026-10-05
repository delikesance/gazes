import { ClaudeView } from "@/components/admin/pages/ClaudeView";
import { ApiErrorBlock } from "@/components/admin/pages/ApiErrorBlock";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { AdminApiError } from "@/lib/admin/api";
import { adminGetServer } from "@/lib/admin/server";
import type { AdminActions, AdminApprovalsList, AdminEnvelope, AdminKillSwitch, AdminMcpAudit, AdminMcpTools } from "@/lib/admin/types";

export default async function Page() {
  let tools: AdminEnvelope<AdminMcpTools>;
  let actions: AdminEnvelope<AdminActions>;
  let approvals: AdminEnvelope<AdminApprovalsList>;
  let kill: AdminEnvelope<AdminKillSwitch>;
  let audit: AdminEnvelope<AdminMcpAudit>;
  try {
    [tools, actions, approvals, kill, audit] = await Promise.all([
      adminGetServer<AdminMcpTools>("/mcp/tools"),
      adminGetServer<AdminActions>("/ops/actions"),
      adminGetServer<AdminApprovalsList>("/approvals", { params: { limit: 50 } }),
      adminGetServer<AdminKillSwitch>("/ops/kill-switch"),
      adminGetServer<AdminMcpAudit>("/mcp/audit", { params: { limit: 50 } }),
    ]);
  } catch (error) {
    if (!(error instanceof AdminApiError)) throw error;
    return (
      <>
        <AdminPageHeader eyebrow="Pilotage automatique" title="Claude et MCP" periods={false} />
        <ApiErrorBlock error={error} />
      </>
    );
  }
  return (
    <ClaudeView
      tools={tools.data}
      actions={actions.data}
      approvals={approvals.data}
      killSwitch={kill.data}
      audit={audit.data}
      generatedAt={audit.generated_at}
    />
  );
}
