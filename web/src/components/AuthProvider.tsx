"use client";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import * as api from "@/lib/auth";
import { resetLists } from "@/lib/lists";
import { setProgressSyncEnabled, syncProgressOnLogin } from "@/lib/watch-progress";

interface AuthState {
  /** `undefined` while the first /auth/me call is in flight. */
  user: api.AccountUser | null | undefined;
  login: typeof api.login;
  register: typeof api.register;
  recover: typeof api.recoverAccount;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<api.AccountUser | null | undefined>(undefined);

  useEffect(() => {
    let active = true;
    api.fetchMe().then((u) => { if (active) { setUser(u); if (u) void syncProgressOnLogin(u.id); } }, () => { if (active) setUser(null); });
    return () => { active = false; };
  }, []);

  const login = useCallback<typeof api.login>(async (input) => {
    const u = await api.login(input);
    setUser(u);
    void syncProgressOnLogin(u.id);
    return u;
  }, []);
  const register = useCallback<typeof api.register>(async (input) => {
    const registered = await api.register(input);
    const { recoveryCodes: _codes, ...u } = registered;
    void _codes;
    setUser(u);
    void syncProgressOnLogin(u.id);
    return registered;
  }, []);
  const recover = useCallback<typeof api.recoverAccount>(async (input) => {
    const u = await api.recoverAccount(input);
    setUser(u);
    void syncProgressOnLogin(u.id);
    return u;
  }, []);
  const logout = useCallback(async () => { await api.logout().catch(() => {}); setProgressSyncEnabled(false); resetLists(); setUser(null); }, []);

  const value = useMemo(() => ({ user, login, register, recover, logout }), [user, login, register, recover, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside <AuthProvider>");
  return ctx;
}
