"use client";

import { useState, useSyncExternalStore } from "react";
import { accessTokenFromUrl, type SavedLink } from "./links";

const noopSubscribe = () => () => {};

// useHydrated is false during server rendering and the first client render, then true.
export function useHydrated(): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    () => true,
    () => false,
  );
}

// useAccessToken resolves the private-link token for a resource on the client (null while hydrating).
export function useAccessToken(kind: SavedLink["kind"], id: string): string | null {
  return useSyncExternalStore(
    noopSubscribe,
    () => accessTokenFromUrl(kind, id),
    () => null,
  );
}

// useUrlFlag reads a one-off query flag such as ?placed=1 on the client.
export function useUrlFlag(name: string): boolean {
  return useSyncExternalStore(
    noopSubscribe,
    () => new URL(window.location.href).searchParams.get(name) === "1",
    () => false,
  );
}

// useNow returns a timestamp fixed at first render, for relative-time decisions that must stay pure.
export function useNow(): number {
  const [now] = useState(() => Date.now());
  return now;
}
