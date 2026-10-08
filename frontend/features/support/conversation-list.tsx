"use client";

import Link from "next/link";
import { useHydrated } from "@/lib/client-hooks";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { savedLinks, type SavedLink } from "@/lib/links";
import { formatDate } from "@/lib/format";
import type { SupportThread } from "@/lib/types";
import { useSession } from "@/components/providers/session";
import { StatusBadge } from "@/components/ui/status-badge";

const statusLabel = { open: "Waiting for us", pending: "Replied", resolved: "Resolved" } as const;

export function ConversationList() {
  const { user, loading } = useSession();
  const hydrated = useHydrated();
  const links: SavedLink[] = hydrated ? savedLinks().filter((l) => l.kind === "support") : [];
  const q = useQuery({ queryKey: ["my-support"], enabled: Boolean(user?.customerId), queryFn: () => api<SupportThread[]>("/me/support") });

  if (loading) return null;
  if (user?.customerId) {
    if (q.isLoading) return <div className="skeleton" style={{ height: 160 }} />;
    const threads = q.data ?? [];
    if (!threads.length) return <p className="muted">You have no conversations yet.</p>;
    return (
      <section className="stack-sm" aria-labelledby="threads-title">
        <h2 id="threads-title" className="title">
          Your conversations
        </h2>
        <ul className="list-rows">
          {threads.map((t) => (
            <li key={t.id}>
              <Link href={`/support/${t.id}`} className="list-row">
                <span className="stack-xs">
                  <strong>{t.subject}</strong>
                  <span className="small muted">
                    {t.number} · {formatDate(t.lastMessageAt)}
                  </span>
                </span>
                <span className="row">
                  {t.unread ? <span className="badge badge-gold">{t.unread} new</span> : null}
                  <StatusBadge status={t.status} label={statusLabel[t.status]} />
                </span>
              </Link>
            </li>
          ))}
        </ul>
      </section>
    );
  }
  return (
    <section className="stack-sm" aria-labelledby="threads-title">
      <h2 id="threads-title" className="title">
        Conversations on this device
      </h2>
      {links.length ? (
        <ul className="list-rows">
          {links.map((l) => (
            <li key={l.id}>
              <Link href={`/support/${l.id}`} className="list-row">
                <span>{l.number || "Conversation"}</span>
                <span className="small muted">{formatDate(new Date(l.createdAt).toISOString())}</span>
              </Link>
            </li>
          ))}
        </ul>
      ) : (
        <p className="muted small">
          Messages you send from this device appear here. <Link className="link" href="/account/sign-in?next=/support">Sign in</Link> to see every conversation on your account.
        </p>
      )}
    </section>
  );
}
