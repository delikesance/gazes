import { notFound } from "next/navigation";
import { AdminShell } from "@/components/admin/layout/AdminShell";
import { ClaudeView } from "@/components/admin/pages/ClaudeView";
import type { AdminActions, AdminApprovalsList, AdminEnvelope, AdminKillSwitch, AdminMcpAudit, AdminMcpTools } from "@/lib/admin/types";
import toolsJson from "@/lib/admin/real/mcp-tools.json";
import actionsJson from "@/lib/admin/real/actions.json";
import approvalsJson from "@/lib/admin/real/approvals.json";
import killJson from "@/lib/admin/real/kill-switch.json";
import killOnJson from "@/lib/admin/real/kill-switch.on.json";
import auditJson from "@/lib/admin/real/mcp-audit.json";
import "@/app/admin/admin.css";

const tools = toolsJson as AdminEnvelope<AdminMcpTools>;
const actions = actionsJson as AdminEnvelope<AdminActions>;
const approvals = approvalsJson as AdminEnvelope<AdminApprovalsList>;
const audit = auditJson as AdminEnvelope<AdminMcpAudit>;

/** Dev-only preview of "Claude et MCP" with real recorded responses (no network, no guard, no writes). `?suspended=1` shows the kill switch on. */
export default async function Page({ searchParams }: { searchParams: Promise<{ [key: string]: string | string[] | undefined }> }) {
  if (process.env.NODE_ENV === "production") notFound();
  const suspended = (await searchParams).suspended === "1";
  const kill = (suspended ? killOnJson : killJson) as AdminEnvelope<AdminKillSwitch>;
  return (
    <AdminShell>
      <ClaudeView tools={tools.data} actions={actions.data} approvals={approvals.data} killSwitch={kill.data} audit={audit.data} generatedAt={approvals.generated_at} demo />
    </AdminShell>
  );
}
