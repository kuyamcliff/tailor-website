"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useSession } from "@/components/providers/session";
import styles from "./owner.module.css";

type NavItem = { href: string; label: string; perm?: string };
const groups: { title: string; items: NavItem[] }[] = [
  { title: "Today", items: [{ href: "/owner", label: "Overview" }] },
  {
    title: "Work",
    items: [
      { href: "/owner/requests", label: "Requests", perm: "requests.read" },
      { href: "/owner/quotes", label: "Quotes", perm: "quotes.write" },
      { href: "/owner/orders", label: "Orders", perm: "orders.read" },
      { href: "/owner/payments", label: "Payments", perm: "payments.read" },
      { href: "/owner/appointments", label: "Appointments", perm: "appointments.read" },
      { href: "/owner/support", label: "Messages", perm: "support.read" },
      { href: "/owner/customers", label: "Customers", perm: "customers.read" },
    ],
  },
  {
    title: "Catalogue",
    items: [
      { href: "/owner/products", label: "Products", perm: "products.write" },
      { href: "/owner/fabrics", label: "Fabrics", perm: "fabrics.write" },
      { href: "/owner/garments", label: "Garments and fit", perm: "garments.write" },
      { href: "/owner/assets", label: "3D assets", perm: "assets.write" },
    ],
  },
  {
    title: "Site",
    items: [
      { href: "/owner/portfolio", label: "Portfolio", perm: "portfolio.write" },
      { href: "/owner/testimonials", label: "Testimonials", perm: "testimonials.write" },
      { href: "/owner/content", label: "Pages and policies", perm: "content.write" },
    ],
  },
  {
    title: "Business",
    items: [
      { href: "/owner/analytics", label: "Analytics", perm: "analytics.read" },
      { href: "/owner/settings", label: "Settings", perm: "settings.write" },
      { href: "/owner/staff", label: "Staff", perm: "staff.write" },
      { href: "/owner/audit", label: "Audit log", perm: "audit.read" },
    ],
  },
];

export function OwnerShell({ children }: { children: React.ReactNode }) {
  const { user, loading, signOut } = useSession();
  const router = useRouter();
  const path = usePathname();
  useEffect(() => {
    if (!loading && !user) router.replace(`/account/sign-in?next=${encodeURIComponent(path)}`);
    else if (!loading && user && !user.isStaff) router.replace("/account");
  }, [loading, user, router, path]);
  const notices = useQuery({
    queryKey: ["owner", "notifications"],
    enabled: Boolean(user?.isStaff),
    refetchInterval: 60_000,
    queryFn: () => api<{ unread: number }>("/owner/notifications"),
  });

  if (loading || !user?.isStaff) return <div className={styles.boot} />;
  const can = (p?: string) => !p || user.permissions.includes(p);
  const active = (href: string) => (href === "/owner" ? path === "/owner" : path.startsWith(href));

  return (
    <div className={styles.shell}>
      <nav className={styles.nav} aria-label="Dashboard">
        <div className={styles.navHead}>
          <Link href="/" className={styles.site}>
            View site
          </Link>
          <span className="small muted">{user.name}</span>
        </div>
        {groups.map((g) => {
          const items = g.items.filter((i) => can(i.perm));
          if (!items.length) return null;
          return (
            <div key={g.title} className={styles.group}>
              <p className={styles.groupTitle}>{g.title}</p>
              <ul>
                {items.map((i) => (
                  <li key={i.href}>
                    <Link href={i.href} aria-current={active(i.href) ? "page" : undefined}>
                      {i.label}
                      {i.href === "/owner" && notices.data?.unread ? (
                        <span className={styles.count}>{notices.data.unread}</span>
                      ) : null}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          );
        })}
        <button
          className={`btn btn-ghost btn-sm ${styles.signout}`}
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

export function PageHead({ title, sub, actions }: { title: string; sub?: React.ReactNode; actions?: React.ReactNode }) {
  return (
    <header className={styles.head}>
      <div>
        <h1 className={styles.title}>{title}</h1>
        {sub ? <p className="small muted">{sub}</p> : null}
      </div>
      {actions ? <div className="row-wrap">{actions}</div> : null}
    </header>
  );
}
