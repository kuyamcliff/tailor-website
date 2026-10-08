"use client";

import { createContext, useContext } from "react";
import type { ContentBlocks, PublicConfig } from "@/lib/types";

const Ctx = createContext<{ config: PublicConfig; content: ContentBlocks } | null>(null);

export function ConfigProvider({
  config,
  content,
  children,
}: {
  config: PublicConfig;
  content: ContentBlocks;
  children: React.ReactNode;
}) {
  return <Ctx.Provider value={{ config, content }}>{children}</Ctx.Provider>;
}

export function useConfig() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useConfig outside ConfigProvider");
  return v.config;
}

export function useContent() {
  const v = useContext(Ctx);
  if (!v) throw new Error("useContent outside ConfigProvider");
  return v.content;
}
