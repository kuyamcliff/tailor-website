"use client";

import Link from "next/link";
import { Suspense } from "react";
import { formatDateTime, humanize } from "@/lib/format";
import { StatusBadge } from "@/components/ui/status-badge";
import { Price } from "@/components/ui/price";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";

type Row = {
  id: string;
  orderId: string;
  orderNumber: string;
  customer: string | null;
  purpose: string;
  provider: string;
  amountMinor: number;
  currency: string;
  status: string;
  simulated: boolean;
  refundedMinor: number;
  failureMessage: string | null;
  providerTransactionId: string | null;
  createdAt: string;
};

const providerName = (p: string) => (p === "mtn" ? "MTN MoMo" : p === "orange" ? "Orange Money" : humanize(p));

export function OwnerPayments() {
  return (
    <>
      <PageHead title="Payments" sub="Every payment attempt. Statuses come from the provider, never from the customer's browser." />
      <Suspense>
        <OwnerList<Row>
          endpoint="/owner/payments"
          filters={[
            { key: "status", label: "Status", type: "select", options: ["pending", "customer_action_required", "processing", "succeeded", "failed", "expired", "cancelled", "refunded", "partially_refunded"].map((s) => [s, humanize(s)]) },
            { key: "provider", label: "Method", type: "select", options: [["mtn", "MTN MoMo"], ["orange", "Orange Money"], ["manual", "Recorded by staff"]] },
          ]}
          rowKey={(r) => r.id}
          empty="No payments match."
          columns={[
            { label: "Date", cell: (r) => formatDateTime(r.createdAt) },
            { label: "Order", cell: (r) => <Link className="link" href={`/owner/orders/${r.orderId}`}>{r.orderNumber}</Link> },
            { label: "Customer", cell: (r) => r.customer ?? "" },
            {
              label: "Method",
              cell: (r) => (
                <>
                  {providerName(r.provider)}
                  {r.simulated ? <span className="badge badge-warning" style={{ marginLeft: 6 }}>Test, no money moved</span> : null}
                </>
              ),
            },
            { label: "For", cell: (r) => humanize(r.purpose) },
            { label: "Amount", cell: (r) => <Price minor={r.amountMinor} currency={r.currency} />, className: "tabular" },
            { label: "Status", cell: (r) => <>{<StatusBadge status={r.status} />}{r.failureMessage ? <div className="tiny muted">{r.failureMessage}</div> : null}</> },
            { label: "Reference", cell: (r) => <span className="tiny muted">{r.providerTransactionId ?? ""}</span> },
          ]}
        />
      </Suspense>
    </>
  );
}
