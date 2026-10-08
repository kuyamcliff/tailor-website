"use client";

import { formatMoney } from "@/lib/money";
import { useConfig } from "@/components/providers/config";

export function Price({ minor, maxMinor, currency, className }: { minor: number; maxMinor?: number; currency?: string; className?: string }) {
  const cfg = useConfig();
  const cur = currency ?? cfg.business.currency;
  const loc = cfg.business.locale || "fr-CM";
  return (
    <span className={`tabular ${className ?? ""}`}>
      {maxMinor && maxMinor !== minor ? `From ${formatMoney(minor, cur, loc)}` : formatMoney(minor, cur, loc)}
    </span>
  );
}
