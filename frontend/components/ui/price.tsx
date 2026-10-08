"use client";

import { formatMoney } from "@/lib/money";
import { useConfig } from "@/components/providers/config";

export function Price({ minor, maxMinor, currency, className }: { minor: number; maxMinor?: number; currency?: string; className?: string }) {
  const cfg = useConfig();
  const cur = currency ?? cfg.business.currency;
  return (
    <span className={`tabular ${className ?? ""}`}>
      {maxMinor && maxMinor !== minor ? `From ${formatMoney(minor, cur)}` : formatMoney(minor, cur)}
    </span>
  );
}
