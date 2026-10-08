import type { Metadata } from "next";
import { Suspense } from "react";
import { ForgotPasswordForm } from "@/features/account/auth-forms";
import styles from "@/features/account/auth.module.css";

export const metadata: Metadata = { title: "Reset your password", robots: { index: false } };

export default function Page() {
  return (
    <div className={styles.wrap}>
      <Suspense fallback={<div className="skeleton" style={{ width: "min(100%, 440px)", height: 420 }} />}>
        <ForgotPasswordForm />
      </Suspense>
    </div>
  );
}
