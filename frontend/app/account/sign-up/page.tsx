import type { Metadata } from "next";
import { Suspense } from "react";
import { SignUpForm } from "@/features/account/auth-forms";
import styles from "@/features/account/auth.module.css";

export const metadata: Metadata = { title: "Create an account", robots: { index: false } };

export default function Page() {
  return (
    <div className={styles.wrap}>
      <Suspense fallback={<div className="skeleton" style={{ width: "min(100%, 440px)", height: 420 }} />}>
        <SignUpForm />
      </Suspense>
    </div>
  );
}
