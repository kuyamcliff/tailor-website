"use client";

import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

// Wishlist and comparison live on the device; the wishlist is merged into the account on sign-in.

export type SavedItem = {
  type: "product" | "fabric" | "design";
  id: string;
  slug?: string;
  name: string;
  image?: string | null;
};

type SavedState = {
  wishlist: SavedItem[];
  compare: SavedItem[];
  toggleWish: (item: SavedItem) => void;
  isWished: (id: string) => boolean;
  toggleCompare: (item: SavedItem) => boolean;
  isCompared: (id: string) => boolean;
  clearCompare: () => void;
};

export const MAX_COMPARE = 4;

export const useSaved = create<SavedState>()(
  persist(
    (set, get) => ({
      wishlist: [],
      compare: [],
      toggleWish: (item) =>
        set((s) => ({
          wishlist: s.wishlist.some((w) => w.id === item.id)
            ? s.wishlist.filter((w) => w.id !== item.id)
            : [item, ...s.wishlist],
        })),
      isWished: (id) => get().wishlist.some((w) => w.id === id),
      toggleCompare: (item) => {
        const s = get();
        if (s.compare.some((c) => c.id === item.id)) {
          set({ compare: s.compare.filter((c) => c.id !== item.id) });
          return true;
        }
        if (s.compare.filter((c) => c.type === item.type).length >= MAX_COMPARE) return false;
        set({ compare: [...s.compare, item] });
        return true;
      },
      isCompared: (id) => get().compare.some((c) => c.id === id),
      clearCompare: () => set({ compare: [] }),
    }),
    { name: "atelier.saved", storage: createJSONStorage(() => localStorage) },
  ),
);
