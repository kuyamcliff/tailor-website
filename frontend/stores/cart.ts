"use client";

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

// The bag holds variant references and display snapshots only. Prices shown here are for display;
// checkout re-prices every item on the server and reports any change before payment.

export type CartLine = {
  variantId: string;
  productSlug: string;
  name: string;
  size: string;
  color: string;
  image: string | null;
  unitPriceMinor: number;
  quantity: number;
  requiresFitting: boolean;
};

type CartState = {
  lines: CartLine[];
  open: boolean;
  add: (line: CartLine) => void;
  setQuantity: (variantId: string, quantity: number) => void;
  remove: (variantId: string) => void;
  clear: () => void;
  setOpen: (open: boolean) => void;
};

export const useCart = create<CartState>()(
  persist(
    (set) => ({
      lines: [],
      open: false,
      add: (line) =>
        set((s) => {
          const existing = s.lines.find((l) => l.variantId === line.variantId);
          if (existing) {
            return {
              open: true,
              lines: s.lines.map((l) =>
                l.variantId === line.variantId ? { ...l, quantity: Math.min(20, l.quantity + line.quantity) } : l,
              ),
            };
          }
          return { open: true, lines: [...s.lines, line] };
        }),
      setQuantity: (variantId, quantity) =>
        set((s) => ({
          lines: s.lines.map((l) =>
            l.variantId === variantId ? { ...l, quantity: Math.max(1, Math.min(20, quantity)) } : l,
          ),
        })),
      remove: (variantId) => set((s) => ({ lines: s.lines.filter((l) => l.variantId !== variantId) })),
      clear: () => set({ lines: [] }),
      setOpen: (open) => set({ open }),
    }),
    {
      name: "atelier.cart",
      storage: createJSONStorage(() => localStorage),
      partialize: (s) => ({ lines: s.lines }),
    },
  ),
);

export function cartCount(lines: CartLine[]) {
  return lines.reduce((n, l) => n + l.quantity, 0);
}

export function cartSubtotal(lines: CartLine[]) {
  return lines.reduce((n, l) => n + l.unitPriceMinor * l.quantity, 0);
}
