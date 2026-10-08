import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { serverApiOr } from "@/lib/server";
import { getConfig } from "@/lib/server-data";
import type { GarmentType } from "@/lib/types";
import { Studio } from "@/features/studio/studio";

export const metadata: Metadata = {
  title: "Fitting studio",
  description: "Design your garment in 3D: choose the cloth and every detail, add your measurements and see an estimate of the fit before requesting a quote.",
  alternates: { canonical: "/studio" },
};

export default async function StudioPage() {
  const [cfg, garments] = await Promise.all([getConfig(), serverApiOr<GarmentType[]>("/garments", [], 60)]);
  if (!cfg.flags.studio || !garments.some((g) => g.studioEnabled))
    return (
      <div className="container-narrow section-tight stack-lg">
        <h1 className="display-2">The fitting studio is closed for now</h1>
        <p className="lede">You can still describe the garment you want and we will reply with a quote.</p>
        <Link className="btn btn-primary" href="/custom-tailor/request">
          Start a custom request
        </Link>
      </div>
    );
  return (
    <>
      <h1 className="visually-hidden">Fitting studio</h1>
      <Suspense fallback={<div style={{ minHeight: "80dvh" }} />}>
        <Studio garments={garments} />
      </Suspense>
    </>
  );
}
