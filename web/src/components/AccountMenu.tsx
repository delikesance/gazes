"use client";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { ChevronDown, Clock, Download, Heart, Megaphone, LogOut, ShieldCheck, Trash2, User as UserIcon } from "lucide-react";
import { useI18n } from "@/lib/i18n";
import { useAuth } from "./AuthProvider";
import { eraseMyWatchLog, exportMyData } from "@/lib/my-data";
import { useBackToClose } from "@/lib/layer-history";

/** Header account area: sign-in/up buttons when signed out, avatar menu when signed in. */
export function AccountMenu() {
  const { t } = useI18n();
  const { user, logout } = useAuth();
  const pathname = usePathname();
  const router = useRouter();
  // Open state is tied to the page it was opened on, so navigating closes it without an effect.
  const [openPath, setOpenPath] = useState<string | null>(null);
  const open = openPath === pathname;
  const setOpen = (value: boolean | ((current: boolean) => boolean)) => setOpenPath((current) => ((typeof value === "function" ? value(current === pathname) : value) ? pathname : null));
  const root = useRef<HTMLDivElement>(null);
  // On a phone the menu is a bottom sheet: the system back gesture closes it.
  useBackToClose(open, () => setOpenPath(null));

  useEffect(() => {
    if (!open) return;
    const onPointer = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpenPath(null); };
    const onKey = (event: KeyboardEvent) => { if (event.key === "Escape") setOpenPath(null); };
    document.addEventListener("pointerdown", onPointer);
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("pointerdown", onPointer); document.removeEventListener("keydown", onKey); };
  }, [open]);

  if (user === undefined) return <span className="account-placeholder" aria-hidden="true" />;

  if (!user) {
    const onLogin = pathname === "/login", onRegister = pathname === "/register";
    return (
      <div className="account-actions">
        {!onLogin && <Link href="/login" className="clay clay-secondary clay-sm account-text">{t("Se connecter")}</Link>}
        {!onRegister && <Link href="/register" className="clay clay-primary clay-sm account-text">{t("S’inscrire")}</Link>}
        <Link href={onLogin ? "/register" : "/login"} className="account-icon" aria-label={t(onLogin ? "S’inscrire" : "Se connecter")}><UserIcon size={18} aria-hidden="true" /></Link>
      </div>
    );
  }

  return (
    <div className="account-actions" ref={root}>
      {user.role === "admin" && (
        <>
          <Link href="/admin" className="clay clay-secondary clay-sm account-text"><ShieldCheck size={15} aria-hidden="true" />{t("Admin")}</Link>
          <Link href="/admin" className="account-icon" aria-label={t("Panneau d’administration")}><ShieldCheck size={18} aria-hidden="true" /></Link>
        </>
      )}
      <button type="button" className="account-pill" aria-haspopup="menu" aria-expanded={open} aria-label={t("Compte")} onClick={() => setOpen((v) => !v)}>
        <span className="account-avatar" aria-hidden="true">{user.pseudo.slice(0, 1).toUpperCase()}</span>
        <span className="account-name">{user.pseudo}</span>
        <ChevronDown size={14} aria-hidden="true" className="account-chevron" />
      </button>
      {open && <div className="account-backdrop" aria-hidden="true" onClick={() => setOpenPath(null)} />}
      {open && (
        <div className="account-menu" role="menu">
          <div className="account-identity">
            <div>{user.pseudo}</div>
            {user.email && <div>{user.email}</div>}
          </div>
          <Link href="/history" role="menuitem" className="account-row"><Clock size={16} aria-hidden="true" />{t("Bibliothèque")}</Link>
          <Link href="/changelog" role="menuitem" className="account-row"><Megaphone size={16} aria-hidden="true" />{t("Nouveautés")}</Link>
          <Link href="/soutenir" role="menuitem" className="account-row"><Heart size={16} aria-hidden="true" />{t("Soutenir")}</Link>
          <Link href="/privacy" role="menuitem" className="account-row"><ShieldCheck size={16} aria-hidden="true" />{t("Confidentialité")}</Link>
          <button type="button" role="menuitem" className="account-row" onClick={async () => { setOpen(false); try { await exportMyData(user.pseudo); } catch { window.alert(t("Impossible d’exporter vos données pour le moment.")); } }}>
            <Download size={16} aria-hidden="true" />{t("Exporter mes données")}
          </button>
          <button type="button" role="menuitem" className="account-row account-danger" onClick={async () => {
            setOpen(false);
            if (!window.confirm(t("Effacer votre journal de visionnage et vos « pas intéressé » ? Vos suggestions repartiront de zéro. Cette action est définitive."))) return;
            try { await eraseMyWatchLog(); } catch { window.alert(t("Impossible d’effacer vos données pour le moment.")); }
          }}>
            <Trash2 size={16} aria-hidden="true" />{t("Effacer mon journal")}
          </button>
          <div className="account-sep" />
          <button type="button" role="menuitem" className="account-row account-danger" onClick={async () => { setOpen(false); await logout(); router.refresh(); }}>
            <LogOut size={16} aria-hidden="true" />{t("Se déconnecter")}
          </button>
        </div>
      )}
    </div>
  );
}
