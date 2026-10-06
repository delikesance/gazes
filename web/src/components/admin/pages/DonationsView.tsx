"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { AdminApiError, adminWrite } from "@/lib/admin/api";
import type { AdminDonations } from "@/lib/admin/types";
import { AdminPageHeader } from "@/components/admin/layout/AdminPageHeader";
import { DataTable, EmptyState, KpiCard, SectionCard } from "@/components/admin/cards";
import type { ColumnDef } from "@/components/admin/cards";
import { Button, Chip, StatTile } from "@/components/admin/ui";
import { TextField } from "@/components/admin/ui/TextField";
import { dateTimeFr } from "./fr-date";
import { MONO_LABEL, MUTED_TEXT, SURFACE, WRAP_ROW } from "./styles";
import { donationKpis, donationRow, donorText, eur, parseEuroInput, parseUserId, publicText, PROVIDER_LABEL, STATUS_LABEL } from "./donations.logic";

export interface DonationsViewProps {
  data: AdminDonations;
  generatedAt: string;
}

const COLUMNS: ColumnDef[] = [
  { key: "donor", label: "Donateur", type: "text" },
  { key: "when", label: "Date (UTC)", type: "mono" },
  { key: "provider", label: "Moyen", type: "text", muted: true },
  { key: "amount", label: "Montant", type: "number", unit: "€", decimals: 2 },
  { key: "status", label: "Statut", type: "status", kind: "chip" },
  { key: "shown", label: "Vu par le public", type: "text", muted: true },
];

type Notice = { tone: "ok" | "error"; text: string } | null;

const FIELDSET: React.CSSProperties = { display: "flex", flexWrap: "wrap", gap: 12, alignItems: "flex-end" };

export function DonationsView({ data, generatedAt }: DonationsViewProps) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [selected, setSelected] = useState<string>("");
  const [notice, setNotice] = useState<Notice>(null);
  const [linkInput, setLinkInput] = useState("");
  const [nameInput, setNameInput] = useState("");
  const [add, setAdd] = useState({ amount: "", label: "", user: "", name: "" });
  const [busy, setBusy] = useState(false);

  const kpis = donationKpis(data.summary);
  const rows = data.donations.map(donationRow);
  const current = data.donations.find((d) => d.id === selected) ?? null;

  const run = async (fn: () => Promise<unknown>, ok: string) => {
    setBusy(true);
    try {
      await fn();
      setNotice({ tone: "ok", text: ok });
      startTransition(() => router.refresh());
    } catch (e) {
      setNotice({ tone: "error", text: e instanceof AdminApiError ? `${e.message} (code ${e.code})` : "Erreur inattendue" });
    } finally {
      setBusy(false);
    }
  };

  const link = (userId: number | null) => current && run(() => adminWrite("POST", `/donations/${current.id}/link`, { user_id: userId }), userId === null ? "Compte détaché." : "Don rattaché au compte.");
  const visibility = (named: boolean) =>
    current && run(() => adminWrite("POST", `/donations/${current.id}/visibility`, named ? { visibility: "named", display_name: nameInput } : { visibility: "anonymous" }), named ? "Nom affiché publiquement." : "Nom masqué : le don est anonyme.");

  const addCents = parseEuroInput(add.amount);
  const addUser = parseUserId(add.user);
  const addValid = addCents !== null && addUser !== undefined;
  const submitAdd = () => {
    if (!addValid) return;
    void run(async () => {
      await adminWrite("POST", "/donations", {
        amount_cents: addCents,
        donor_label: add.label,
        user_id: addUser,
        ...(add.name.trim() ? { visibility: "named", display_name: add.name } : {}),
      });
      setAdd({ amount: "", label: "", user: "", name: "" });
    }, "Don enregistré.");
  };

  return (
    <>
      <AdminPageHeader
        eyebrow="Soutien"
        title="Dons"
        subtitle={`Instantané du ${dateTimeFr(generatedAt)} · qui donne quoi, et ce que le public en voit`}
        periods={false}
        badges={<Chip label="Session admin uniquement" tone="accent" />}
      />

      <section aria-label="Chiffres clés" style={WRAP_ROW}>
        {kpis.map((k) => (
          <KpiCard key={k.label} label={k.label} value={k.value} note={k.note} showSpark={false} basis={180} />
        ))}
      </section>

      {notice ? (
        <p role={notice.tone === "error" ? "alert" : "status"} style={{ margin: 0, fontSize: 13, color: notice.tone === "error" ? "#fecaca" : "#9b8afb" }}>
          {notice.text}
        </p>
      ) : null}

      <div style={{ display: "flex" }}>
        <DataTable
          id="dons"
          title="Dons"
          subtitle="Du plus récent au plus ancien · cliquez un donateur pour agir sur son don"
          columns={COLUMNS}
          rows={rows}
          rowKey="id"
          sortKey="when"
          sortDir="desc"
          selectable
          selectedKey={selected}
          onSelect={(row) => {
            const id = String(row.id);
            setSelected(id);
            setLinkInput("");
            setNameInput("");
          }}
          busy={pending}
          minWidth={860}
          caption="Dons reçus, avec le donateur et ce qui est visible publiquement"
          emptyText="Aucun don pour l'instant."
          footnote="Le public ne voit jamais un montant, un compte ou une référence : seulement un nom que le donateur a choisi d'afficher. Aucun e-mail n'est enregistré."
        />
      </div>

      <section aria-label="Détail du don" style={{ ...SURFACE, gap: 20 }}>
        <SectionCard title="Détail du don" subtitle={current ? donorText(current) : "Aucun don sélectionné"} bare />
        {current ? (
          <>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
              <StatTile label="Montant" value={eur(current.amount_cents, current.currency)} size="sm" basis={150} />
              <StatTile label="Moyen" value={PROVIDER_LABEL[current.provider] ?? current.provider} size="sm" basis={150} />
              <StatTile label="Statut" value={STATUS_LABEL[current.status] ?? current.status} size="sm" basis={150} />
              <StatTile label="Référence" value={current.provider_ref} mono size="sm" basis={150} />
            </div>
            {current.message ? <p style={{ ...MUTED_TEXT, fontStyle: "italic" }}>« {current.message} »</p> : null}
            <p style={MUTED_TEXT}>Vu par le public : {publicText(current)}</p>

            <div style={{ display: "flex", flexWrap: "wrap", gap: 24 }}>
              <div style={{ flex: "1 1 320px", display: "flex", flexDirection: "column", gap: 12 }}>
                <h3 style={{ ...MONO_LABEL, margin: 0 }}>Compte rattaché</h3>
                <div style={FIELDSET}>
                  <TextField label="Numéro de compte" value={linkInput} onChange={(e) => setLinkInput(e.target.value)} placeholder={current.user_id ? `#${current.user_id}` : "ex. 12"} inputMode="numeric" />
                  <Button variant="secondary" disabled={busy || parseUserId(linkInput) === undefined || linkInput.trim() === ""} onClick={() => link(parseUserId(linkInput) ?? null)}>
                    Rattacher
                  </Button>
                  {current.user_id !== null ? (
                    <Button variant="ghost" disabled={busy} onClick={() => link(null)}>
                      Détacher
                    </Button>
                  ) : null}
                </div>
              </div>

              <div style={{ flex: "1 1 320px", display: "flex", flexDirection: "column", gap: 12 }}>
                <h3 style={{ ...MONO_LABEL, margin: 0 }}>Nom public</h3>
                <div style={FIELDSET}>
                  <TextField label="Nom à afficher" value={nameInput} onChange={(e) => setNameInput(e.target.value)} placeholder={current.display_name || "2 à 24 caractères"} maxLength={24} />
                  <Button variant="secondary" disabled={busy || nameInput.trim().length < 2} onClick={() => visibility(true)}>
                    Afficher
                  </Button>
                  {current.visibility === "named" ? (
                    <Button variant="danger" disabled={busy} onClick={() => visibility(false)}>
                      Masquer le nom
                    </Button>
                  ) : null}
                </div>
              </div>
            </div>
          </>
        ) : (
          <EmptyState title="Sélectionnez un don dans le tableau pour le rattacher à un compte ou masquer son nom." />
        )}
      </section>

      <section aria-label="Saisie manuelle" style={SURFACE}>
        <SectionCard title="Enregistrer un don reçu hors plateforme" subtitle="Virement, Liberapay, espèces : il compte comme reçu immédiatement" bare />
        <div style={FIELDSET}>
          <TextField label="Montant" value={add.amount} onChange={(e) => setAdd({ ...add, amount: e.target.value })} suffix="€" inputMode="decimal" placeholder="5" />
          <TextField label="Libellé du donateur (privé)" value={add.label} onChange={(e) => setAdd({ ...add, label: e.target.value })} maxLength={80} />
          <TextField label="Compte (optionnel)" value={add.user} onChange={(e) => setAdd({ ...add, user: e.target.value })} inputMode="numeric" placeholder="ex. 12" />
          <TextField label="Nom public (optionnel)" value={add.name} onChange={(e) => setAdd({ ...add, name: e.target.value })} maxLength={24} />
          <Button variant="primary" icon="plus" disabled={busy || !addValid} onClick={submitAdd}>
            Enregistrer
          </Button>
        </div>
      </section>
    </>
  );
}
