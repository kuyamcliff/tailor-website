import type { Metadata } from "next";
import { Suspense } from "react";
import { RequestWizard } from "@/features/custom/request-wizard";

export const metadata: Metadata = {
  title: "Request a custom garment",
  description:
    "Describe the garment you want, add your measurements and reference photos, and receive a quote from your tailor.",
  alternates: { canonical: "/custom-tailor/request" },
};

export default function Page() {
  return (
    <Suspense
      fallback={
        <div className="container section-tight">
          <div className="skeleton" style={{ height: 480 }} />
        </div>
      }
    >
      <RequestWizard />
    </Suspense>
  );
}
