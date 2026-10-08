"use client";

import { useEffect } from "react";
import { reportMetric } from "@/lib/telemetry";

export function WebVitals() {
  useEffect(() => {
    let cancelled = false;
    void import("web-vitals").then(({ onCLS, onINP, onLCP, onFCP, onTTFB }) => {
      if (cancelled) return;
      const report = (m: { name: string; value: number; rating: string }) =>
        reportMetric(m.name, m.value, { rating: m.rating });
      onCLS(report);
      onINP(report);
      onLCP(report);
      onFCP(report);
      onTTFB(report);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  return null;
}
