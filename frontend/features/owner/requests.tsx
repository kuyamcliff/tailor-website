"use client";

import { Suspense } from "react";
import { formatDate, humanize, requestLabels } from "@/lib/format";
import { StatusBadge } from "@/components/ui/status-badge";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";

type Row = {
  id: string;
  number: string;
  status: string;
  garment: string;
  occasion: string;
  urgency: string;
  desiredDate: string | null;
  customer: string;
  measurementMode: string;
  references: number;
  createdAt: string;
};

export function OwnerRequests() {
  return (
    <>
      <PageHead title="Requests" sub="Custom garment requests, newest first within each status." />
      <Suspense>
        <OwnerList<Row>
          endpoint="/owner/requests"
          filters={[
            { key: "q", label: "Search", type: "search" },
            { key: "status", label: "Status", type: "select", options: Object.entries(requestLabels) },
            { key: "urgency", label: "Urgency", type: "select", options: [["standard", "Standard"], ["soon", "Soon"], ["urgent", "Urgent"]] },
          ]}
          rowKey={(r) => r.id}
          href={(r) => `/owner/requests/${r.id}`}
          empty="No requests match."
          columns={[
            { label: "Request", cell: (r) => r.number },
            { label: "Customer", cell: (r) => r.customer },
            { label: "Garment", cell: (r) => r.garment },
            { label: "Occasion", cell: (r) => humanize(r.occasion) },
            { label: "Needed by", cell: (r) => (r.desiredDate ? formatDate(r.desiredDate, "short") : "") },
            { label: "Measurements", cell: (r) => humanize(r.measurementMode) },
            { label: "Photos", cell: (r) => r.references || "", className: "tabular" },
            { label: "Status", cell: (r) => <StatusBadge status={r.status} label={requestLabels[r.status]} /> },
            { label: "Received", cell: (r) => formatDate(r.createdAt, "short") },
          ]}
        />
      </Suspense>
    </>
  );
}
