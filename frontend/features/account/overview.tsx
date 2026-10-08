"use client";

import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell } from "lucide-react";
import { api } from "@/lib/api";
import { formatDate, formatDateTime, requestLabels } from "@/lib/format";
import type { Appointment } from "@/lib/types";
import { useSession } from "@/components/providers/session";
import { useConfig } from "@/components/providers/config";
import { StatusBadge } from "@/components/ui/status-badge";
import { Price } from "@/components/ui/price";

export type MyOrder = {
  id: string;
  number: string;
  kind: string;
  status: string;
  statusLabel: string;
  paymentStatus: string;
  totalMinor: number;
  currency: string;
  createdAt: string;
  summary: string | null;
};
export type MyRequest = { id: string; number: string; status: string; garment: string; occasion: string; createdAt: string };
type Notice = { id: string; event: string; title: string; body: string; link: string | null; readAt: string | null; createdAt: string };

export function useMyOrders() {
  return useQuery({ queryKey: ["me", "orders"], queryFn: () => api<MyOrder[]>("/me/orders") });
}
export function useMyRequests() {
  return useQuery({ queryKey: ["me", "requests"], queryFn: () => api<MyRequest[]>("/me/requests") });
}
export function useMyAppointments() {
  return useQuery({ queryKey: ["me", "appointments"], queryFn: () => api<Appointment[]>("/me/appointments") });
}

export function OrderRows({ orders }: { orders: MyOrder[] }) {
  return (
    <ul className="list-rows">
      {orders.map((o) => (
        <li key={o.id}>
          <Link className="list-row" href={`/orders/${o.id}`}>
            <span className="stack-xs">
              <strong>{o.summary || `Order ${o.number}`}</strong>
              <span className="small muted">
                {o.number} · {formatDate(o.createdAt)}
              </span>
            </span>
            <span className="row-wrap" style={{ justifyContent: "flex-end" }}>
              <Price minor={o.totalMinor} currency={o.currency} className="tabular" />
              <StatusBadge status={o.status} label={o.statusLabel || undefined} />
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function RequestRows({ requests }: { requests: MyRequest[] }) {
  return (
    <ul className="list-rows">
      {requests.map((r) => (
        <li key={r.id}>
          <Link className="list-row" href={`/requests/${r.id}`}>
            <span className="stack-xs">
              <strong>{r.garment}</strong>
              <span className="small muted">
                {r.number} · {formatDate(r.createdAt)}
                {r.occasion ? ` · ${r.occasion}` : ""}
              </span>
            </span>
            <StatusBadge status={r.status} label={requestLabels[r.status]} />
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function AccountOverview() {
  const { user } = useSession();
  const cfg = useConfig();
  const qc = useQueryClient();
  const orders = useMyOrders();
  const requests = useMyRequests();
  const appts = useMyAppointments();
  const notices = useQuery({ queryKey: ["me", "notifications"], queryFn: () => api<{ items: Notice[]; unread: number }>("/me/notifications") });
  const next = (appts.data ?? []).filter((a) => a.status === "booked" && new Date(a.startsAt) > new Date()).reverse()[0];
  const active = (orders.data ?? []).filter((o) => !["completed", "cancelled", "refunded", "delivered"].includes(o.status));
  const openRequests = (requests.data ?? []).filter((r) => !["converted", "closed"].includes(r.status));

  async function markRead() {
    await api("/me/notifications/read", { method: "POST", body: {} });
    await qc.invalidateQueries({ queryKey: ["me", "notifications"] });
  }

  return (
    <>
      <header className="stack-sm">
        <span className="eyebrow">Your account</span>
        <h1 className="display-2">Welcome back, {user?.name.split(" ")[0]}</h1>
      </header>

      {next ? (
        <section className="panel panel-pad stack-sm" aria-labelledby="next-appt">
          <h2 id="next-appt" className="title">
            Next appointment
          </h2>
          <p className="lede">
            {next.typeLabel}, {formatDateTime(next.startsAt, cfg.business.timezone)}
          </p>
          <Link className="link" href={`/appointments/${next.id}`}>
            Manage appointment
          </Link>
        </section>
      ) : null}

      <section className="stack-sm" aria-labelledby="active-orders">
        <div className="spread">
          <h2 id="active-orders" className="title">
            Orders in progress
          </h2>
          <Link className="link small" href="/account/orders">
            All orders
          </Link>
        </div>
        {orders.isLoading ? (
          <div className="skeleton" style={{ height: 120 }} />
        ) : active.length ? (
          <OrderRows orders={active.slice(0, 5)} />
        ) : (
          <p className="muted">
            Nothing in progress. <Link className="link" href="/shop">Browse the shop</Link> or{" "}
            <Link className="link" href="/custom-tailor">start a bespoke request</Link>.
          </p>
        )}
      </section>

      {openRequests.length ? (
        <section className="stack-sm" aria-labelledby="open-requests">
          <h2 id="open-requests" className="title">
            Bespoke requests
          </h2>
          <RequestRows requests={openRequests.slice(0, 5)} />
        </section>
      ) : null}

      <section className="stack-sm" aria-labelledby="updates">
        <div className="spread">
          <h2 id="updates" className="title row">
            <Bell size={18} aria-hidden /> Updates
            {notices.data?.unread ? <span className="badge badge-gold">{notices.data.unread} new</span> : null}
          </h2>
          {notices.data?.unread ? (
            <button className="btn btn-ghost btn-sm" onClick={markRead}>
              Mark all as read
            </button>
          ) : null}
        </div>
        {notices.isLoading ? (
          <div className="skeleton" style={{ height: 120 }} />
        ) : notices.data?.items.length ? (
          <ul className="list-rows">
            {notices.data.items.slice(0, 10).map((n) => {
              const inner = (
                <>
                  <span className="stack-xs">
                    <strong style={{ fontWeight: n.readAt ? 500 : 700 }}>{n.title}</strong>
                    <span className="small muted">{n.body}</span>
                  </span>
                  <span className="small faint" style={{ whiteSpace: "nowrap" }}>
                    {formatDate(n.createdAt, "en-GB", { day: "numeric", month: "short" })}
                  </span>
                </>
              );
              return (
                <li key={n.id}>
                  {n.link && n.link.startsWith("/") ? (
                    <Link className="list-row" href={n.link}>
                      {inner}
                    </Link>
                  ) : (
                    <div className="list-row">{inner}</div>
                  )}
                </li>
              );
            })}
          </ul>
        ) : (
          <p className="muted">No updates yet.</p>
        )}
      </section>
    </>
  );
}
