import type { Metadata } from "next";
import { CompareView } from "@/features/shop/compare-view";

export const metadata: Metadata = { title: "Compare", robots: { index: false } };

export default function ComparePage() {
  return (
    <div className="container section-tight">
      <h1 className="display-2" style={{ marginBottom: 24 }}>
        Compare
      </h1>
      <CompareView />
    </div>
  );
}
