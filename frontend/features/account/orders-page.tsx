"use client";

import Link from "next/link";
import { useConfig } from "@/components/providers/config";
import { StatusBadge } from "@/components/ui/status-badge";
import { formatDateTime } from "@/lib/format";
import { OrderRows, RequestRows, useMyAppointments, useMyOrders, useMyRequests } from "./overview";

export function AccountOrders() {
  const cfg = useConfig();
  const orders = useMyOrders();
  const requests = useMyRequests();
  const appts = useMyAppointments();
  return (
    <>
      <header className="stack-sm">
        <h1 className="display-2">Orders and requests</h1>
      </header>
      <section className="stack-sm" aria-labelledby="orders-h">
        <h2 id="orders-h" className="title">
          Orders
        </h2>
        {orders.isLoading ? (
          <div className="skeleton" style={{ height: 160 }} />
        ) : orders.data?.length ? (
          <OrderRows orders={orders.data} />
        ) : (
          <p className="muted">No orders yet.</p>
        )}
      </section>
      <section className="stack-sm" aria-labelledby="requests-h">
        <div className="spread">
          <h2 id="requests-h" className="title">
            Bespoke requests and quotes
          </h2>
          <Link className="link small" href="/custom-tailor/request">
            New request
          </Link>
        </div>
        {requests.isLoading ? (
          <div className="skeleton" style={{ height: 120 }} />
        ) : requests.data?.length ? (
          <RequestRows requests={requests.data} />
        ) : (
          <p className="muted">No requests yet.</p>
        )}
      </section>
      <section className="stack-sm" aria-labelledby="appts-h">
        <div className="spread">
          <h2 id="appts-h" className="title">
            Appointments
          </h2>
          <Link className="link small" href="/appointments">
            Book
          </Link>
        </div>
        {appts.isLoading ? (
          <div className="skeleton" style={{ height: 120 }} />
        ) : appts.data?.length ? (
          <ul className="list-rows">
            {appts.data.map((a) => (
              <li key={a.id}>
                <Link className="list-row" href={`/appointments/${a.id}`}>
                  <span className="stack-xs">
                    <strong>{a.typeLabel}</strong>
                    <span className="small muted">
                      {a.number} · {formatDateTime(a.startsAt, cfg.business.timezone)}
                    </span>
                  </span>
                  <StatusBadge status={a.status} />
                </Link>
              </li>
            ))}
          </ul>
        ) : (
          <p className="muted">No appointments yet.</p>
        )}
      </section>
    </>
  );
}
