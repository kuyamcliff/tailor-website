import type { Metadata } from "next";
import { Suspense } from "react";
import { SignInForm } from "@/features/account/auth-forms";
import styles from "@/features/account/auth.module.css";

export const metadata: Metadata = { title: "Sign in", robots: { index: false } };

export default function Page() {
  return (
    <div className={styles.wrap}>
      <Suspense fallback={<div className="skeleton" style={{ width: "min(100%, 440px)", height: 420 }} />}>
        <SignInForm />
      </Suspense>
    </div>
  );
}
