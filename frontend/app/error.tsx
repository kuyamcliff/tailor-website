"use client";

import { useEffect } from "react";
import Link from "next/link";
import { reportError } from "@/lib/telemetry";

export default function ErrorPage({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => {
    reportError(error, { boundary: "route", digest: error.digest });
  }, [error]);
  return (
    <section className="section">
      <div className="container-narrow stack-lg">
        <span className="eyebrow">Something went wrong</span>
        <h1 className="display-2">This page could not load.</h1>
        <p className="lede">Please try again. If the problem continues, contact us and we will help.</p>
        <div className="row-wrap">
          <button className="btn btn-primary" onClick={() => reset()}>
            Try again
          </button>
          <Link href="/contact" className="btn">
            Contact us
          </Link>
        </div>
        {error.digest ? <p className="tiny faint">Reference: {error.digest}</p> : null}
      </div>
    </section>
  );
}
