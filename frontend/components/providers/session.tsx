"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { api, setCsrfToken } from "@/lib/api";
import type { Me } from "@/lib/types";
import { useSaved } from "@/stores/saved";

type Session = {
  user: Me | null;
  loading: boolean;
  refresh: () => Promise<Me | null>;
  signIn: (identifier: string, password: string) => Promise<Me>;
  signUp: (input: {
    name: string;
    email: string;
    phone?: string;
    password: string;
    marketingConsent: boolean;
  }) => Promise<Me>;
  signOut: () => Promise<void>;
};

const Ctx = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<Me | null>(null);
  const [loading, setLoading] = useState(true);

  const apply = useCallback((u: Me | null) => {
    setUser(u);
    setCsrfToken(u?.csrfToken ?? "");
    return u;
  }, []);

  const refresh = useCallback(async () => {
    try {
      const r = await api<{ user: Me | null }>("/auth/me");
      return apply(r.user);
    } catch {
      return apply(null);
    } finally {
      setLoading(false);
    }
  }, [apply]);

  useEffect(() => {
    // Load the session from the API once on mount (external system sync).
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh();
  }, [refresh]);

  // After sign-in, move this device's guest designs, uploads and wishlist into the account.
  const afterSignIn = useCallback(async (u: Me) => {
    if (!u.customerId) return;
    try {
      await api("/me/claim-guest", { method: "POST", body: {} });
      const items = useSaved.getState().wishlist.map((w) => ({ itemType: w.type, itemId: w.id }));
      if (items.length) await api("/me/wishlist", { method: "POST", body: { items } });
    } catch {
      // Non-critical: the data stays on the device and can be claimed next time.
    }
  }, []);

  const signIn = useCallback(
    async (identifier: string, password: string) => {
      const r = await api<{ user: Me }>("/auth/signin", { method: "POST", body: { identifier, password } });
      apply(r.user);
      void afterSignIn(r.user);
      return r.user;
    },
    [apply, afterSignIn],
  );

  const signUp = useCallback(
    async (input: { name: string; email: string; phone?: string; password: string; marketingConsent: boolean }) => {
      const r = await api<{ user: Me }>("/auth/signup", { method: "POST", body: input });
      apply(r.user);
      void afterSignIn(r.user);
      return r.user;
    },
    [apply, afterSignIn],
  );

  const signOut = useCallback(async () => {
    try {
      await api("/auth/signout", { method: "POST", body: {} });
    } finally {
      apply(null);
    }
  }, [apply]);

  const value = useMemo(
    () => ({ user, loading, refresh, signIn, signUp, signOut }),
    [user, loading, refresh, signIn, signUp, signOut],
  );
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useSession() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useSession outside SessionProvider");
  return v;
}
