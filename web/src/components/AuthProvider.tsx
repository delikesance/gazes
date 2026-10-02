"use client";
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import * as api from "@/lib/auth";
import { setProgressSyncEnabled, syncProgressOnLogin } from "@/lib/watch-progress";

interface AuthState {
  /** `undefined` while the first /auth/me call is in flight. */
  user: api.AccountUser | null | undefined;
  login: typeof api.login;
  register: typeof api.register;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<api.AccountUser | null | undefined>(undefined);

  useEffect(() => {
    let active = true;
    api.fetchMe().then((u) => { if (active) { setUser(u); if (u) void syncProgressOnLogin(); } }, () => { if (active) setUser(null); });
    return () => { active = false; };
  }, []);

  const login = useCallback<typeof api.login>(async (input) => {
    const u = await api.login(input);
    setUser(u);
    void syncProgressOnLogin();
    return u;
  }, []);
  const register = useCallback<typeof api.register>(async (input) => {
    const u = await api.register(input);
    setUser(u);
    void syncProgressOnLogin();
    return u;
  }, []);
  const logout = useCallback(async () => { await api.logout().catch(() => {}); setProgressSyncEnabled(false); setUser(null); }, []);

  const value = useMemo(() => ({ user, login, register, logout }), [user, login, register, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used inside <AuthProvider>");
  return ctx;
}
