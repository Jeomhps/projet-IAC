import React, { createContext, useContext, useMemo, useState, useEffect } from "react";
import { apiGet } from "./client";

type AuthState = {
  token: string | null;
  roles: string[];
  user: string | null;
};

type AuthContextType = AuthState & {
  login: (username: string, password: string) => Promise<boolean>;
  logout: () => void;
};

const AuthContext = createContext<AuthContextType | undefined>(undefined);

const STORAGE_KEY = "projet-iac-token";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(() => sessionStorage.getItem(STORAGE_KEY));
  const [roles, setRoles] = useState<string[]>([]);
  const [user, setUser] = useState<string | null>(null);

  async function fetchWhoAmI(t: string) {
    try {
      const res = await apiGet("/whoami", t);
      setRoles(res.roles || []);
      setUser(res.user || null);
    } catch {
      setRoles([]);
      setUser(null);
    }
  }

  useEffect(() => {
    if (token) fetchWhoAmI(token);
  }, [token]);

  const value = useMemo<AuthContextType>(() => ({
    token,
    roles,
    user,
    login: async (username: string, password: string) => {
      try {
        const resp = await fetch("/api/login", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ username, password }),
        });
        if (!resp.ok) return false;
        const data = await resp.json();
        const t = data.access_token as string;
        if (!t) return false;
        sessionStorage.setItem(STORAGE_KEY, t);
        setToken(t);
        await fetchWhoAmI(t);
        return true;
      } catch {
        return false;
      }
    },
    logout: () => {
      sessionStorage.removeItem(STORAGE_KEY);
      setToken(null);
      setRoles([]);
      setUser(null);
    }
  }), [token, roles, user]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}

export function hasRole(roles: string[], role: string) {
  return roles.includes(role);
}
