"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { useSession } from "@/components/providers/session";
import styles from "./account.module.css";

const nav = [
  ["/account", "Overview"],
  ["/account/orders", "Orders and requests"],
  ["/account/measurements", "Measurements"],
  ["/account/designs", "Saved designs"],
  ["/account/addresses", "Addresses"],
  ["/account/settings", "Settings and privacy"],
] as const;

export function AccountShell({ children }: { children: React.ReactNode }) {
  const { user, loading, signOut } = useSession();
  const router = useRouter();
  const path = usePathname();

  useEffect(() => {
    if (!loading && !user) router.replace(`/account/sign-in?next=${encodeURIComponent(path)}`);
    else if (!loading && user && !user.customerId) router.replace("/owner");
  }, [loading, user, router, path]);

  if (loading || !user?.customerId)
    return (
      <div className="container section-tight">
        <div className="skeleton" style={{ height: 420 }} />
      </div>
    );

  return (
    <div className={`container section-tight ${styles.shell}`}>
      <nav className={styles.nav} aria-label="Account">
        <p className="eyebrow">{user.name}</p>
        <ul>
          {nav.map(([href, label]) => (
            <li key={href}>
              <Link href={href} aria-current={path === href ? "page" : undefined}>
                {label}
              </Link>
            </li>
          ))}
          <li>
            <Link href="/support">Messages</Link>
          </li>
        </ul>
        <button
          className="btn btn-ghost btn-sm"
          onClick={async () => {
            await signOut();
            router.replace("/");
          }}
        >
          Sign out
        </button>
      </nav>
      <div className={styles.main}>{children}</div>
    </div>
  );
}
