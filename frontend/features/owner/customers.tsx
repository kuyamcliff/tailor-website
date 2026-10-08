"use client";

import Link from "next/link";
import { Suspense, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate, humanize, paymentLabels, requestLabels } from "@/lib/format";
import { formatLength } from "@/lib/units";
import type { MeasurementProfile } from "@/lib/types";
import { useToast } from "@/components/providers/toast";
import { StatusBadge } from "@/components/ui/status-badge";
import { Price } from "@/components/ui/price";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Row = {
  id: string;
  name: string;
  phone: string | null;
  email: string | null;
  preferredContact: string;
  hasAccount: boolean;
  orders: number;
  requests: number;
  lastActivity: string | null;
  createdAt: string;
};

export function OwnerCustomers() {
  return (
    <>
      <PageHead title="Customers" />
      <Suspense>
        <OwnerList<Row>
          endpoint="/owner/customers"
          filters={[{ key: "q", label: "Name, phone or email", type: "search" }]}
          rowKey={(r) => r.id}
          href={(r) => `/owner/customers/${r.id}`}
          empty="No customers match."
          columns={[
            { label: "Name", cell: (r) => r.name },
            { label: "Phone", cell: (r) => (r.phone ? `+${r.phone}` : "") },
            { label: "Email", cell: (r) => r.email ?? "" },
            { label: "Account", cell: (r) => (r.hasAccount ? "Yes" : "Guest") },
            { label: "Orders", cell: (r) => r.orders || "", className: "tabular" },
            { label: "Requests", cell: (r) => r.requests || "", className: "tabular" },
            { label: "Last activity", cell: (r) => (r.lastActivity ? formatDate(r.lastActivity, "short") : "") },
          ]}
        />
      </Suspense>
    </>
  );
}

type Detail = {
  customer: Row & { internalNotes: string; marketingConsent: boolean };
  orders: {
    id: string;
    number: string;
    status: string;
    payment_status: string;
    total_minor: number;
    currency: string;
    created_at: string;
  }[];
  requests: {
    id: string;
    number: string;
    status: string;
    garment_type_key: string;
    occasion: string;
    created_at: string;
  }[];
  appointments: { id: string; number: string; type: string; status: string; starts_at: string }[];
  support: { id: string; number: string; subject: string; status: string; last_message_at: string }[];
  designs: { id: string; name: string; garment_type_key: string; updated_at: string }[];
};

export function OwnerCustomer({ id }: { id: string }) {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["owner", "customer", id], queryFn: () => api<Detail>(`/owner/customers/${id}`) });
  const profiles = useQuery({
    queryKey: ["owner", "customer-profiles", id],
    queryFn: () => api<MeasurementProfile[]>(`/owner/customers/${id}/measurement-profiles`),
  });
  const [notes, setNotes] = useState<string | null>(null);
  if (q.isLoading) return <div className="skeleton" style={{ height: 420 }} />;
  if (!q.data) return <p className="notice notice-danger">This customer could not be loaded.</p>;
  const { customer: c } = q.data;
  async function saveNotes() {
    try {
      await api(`/owner/customers/${id}`, {
        method: "PATCH",
        body: { internalNotes: notes ?? "", preferredContact: c.preferredContact },
      });
      toast("Notes saved.");
      setNotes(null);
      await qc.invalidateQueries({ queryKey: ["owner", "customer", id] });
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "Please try again.", "error");
    }
  }
  return (
    <>
      <PageHead
        title={c.name}
        sub={`Customer since ${formatDate(c.createdAt)}${c.hasAccount ? " · has an account" : " · guest"}`}
      />
      <div className={styles.detail}>
        <div className="stack-lg">
          <Related title="Orders" empty="No orders.">
            {q.data.orders.map((o) => (
              <li key={o.id}>
                <Link className="list-row" href={`/owner/orders/${o.id}`}>
                  <span>
                    {o.number} · {formatDate(o.created_at, "short")}
                  </span>
                  <span className="row">
                    <Price minor={o.total_minor} currency={o.currency} />
                    <StatusBadge status={o.status} />
                    <StatusBadge status={o.payment_status} label={paymentLabels[o.payment_status]} />
                  </span>
                </Link>
              </li>
            ))}
          </Related>
          <Related title="Requests" empty="No requests.">
            {q.data.requests.map((r) => (
              <li key={r.id}>
                <Link className="list-row" href={`/owner/requests/${r.id}`}>
                  <span>
                    {r.number} · {humanize(r.garment_type_key)} for {humanize(r.occasion).toLowerCase()}
                  </span>
                  <StatusBadge status={r.status} label={requestLabels[r.status]} />
                </Link>
              </li>
            ))}
          </Related>
          <Related title="Appointments" empty="No appointments.">
            {q.data.appointments.map((a) => (
              <li key={a.id} className="list-row">
                <span>
                  {formatDate(a.starts_at)} · {humanize(a.type)}
                </span>
                <StatusBadge status={a.status} />
              </li>
            ))}
          </Related>
          <Related title="Messages" empty="No messages.">
            {q.data.support.map((t) => (
              <li key={t.id}>
                <Link className="list-row" href={`/owner/support/${t.id}`}>
                  <span>{t.subject}</span>
                  <StatusBadge status={t.status} />
                </Link>
              </li>
            ))}
          </Related>
          <section className="stack-sm" aria-labelledby="meas-h">
            <h2 id="meas-h" className={styles.h2}>
              Measurement profiles
            </h2>
            {(profiles.data ?? []).length ? (
              (profiles.data ?? []).map((p) => (
                <div key={p.id} className="stack-sm">
                  <p className="small" style={{ margin: 0 }}>
                    <strong>{p.name}</strong>{" "}
                    <span className="muted">
                      ·{" "}
                      {p.current
                        ? `${humanize(p.current.source)}, ${formatDate(p.current.createdAt, "short")}`
                        : "no measurements"}
                    </span>
                  </p>
                  {p.current ? (
                    <p className="small muted tabular" style={{ margin: 0 }}>
                      {Object.entries(p.current.valuesMm)
                        .map(([k, mm]) => `${humanize(k)} ${formatLength(mm, p.unit)}`)
                        .join(" · ")}
                    </p>
                  ) : null}
                </div>
              ))
            ) : (
              <p className="small muted">No saved profiles.</p>
            )}
          </section>
        </div>
        <aside className={styles.side}>
          <section className="panel panel-pad stack-sm" aria-labelledby="c-h">
            <h2 id="c-h" className={styles.h2}>
              Contact
            </h2>
            {c.phone ? <a href={`tel:+${c.phone}`}>+{c.phone}</a> : null}
            {c.email ? <a href={`mailto:${c.email}`}>{c.email}</a> : null}
            <span className="small muted">Prefers {humanize(c.preferredContact).toLowerCase()}</span>
            <span className="small muted">
              {c.marketingConsent ? "Agreed to news and offers" : "No marketing consent"}
            </span>
          </section>
          <section className="panel panel-pad stack-sm" aria-labelledby="n-h">
            <h2 id="n-h" className={styles.h2}>
              Internal notes
            </h2>
            <textarea
              className="textarea"
              rows={6}
              aria-label="Internal notes"
              value={notes ?? c.internalNotes}
              onChange={(e) => setNotes(e.target.value)}
            />
            {notes !== null && notes !== c.internalNotes ? (
              <button className="btn btn-sm" onClick={saveNotes}>
                Save notes
              </button>
            ) : null}
          </section>
        </aside>
      </div>
    </>
  );
}

function Related({ title, empty, children }: { title: string; empty: string; children: React.ReactNode[] }) {
  return (
    <section className="stack-sm" aria-label={title}>
      <h2 className={styles.h2}>{title}</h2>
      {children.length ? <ul className="list-rows">{children}</ul> : <p className="small muted">{empty}</p>}
    </section>
  );
}
