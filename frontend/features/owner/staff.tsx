"use client";

import { Suspense, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "@/lib/api";
import { formatDate, formatDateTime, humanize } from "@/lib/format";
import { useSession } from "@/components/providers/session";
import { useToast } from "@/components/providers/toast";
import { OwnerList } from "./owner-list";
import { PageHead } from "./owner-shell";
import styles from "./tables.module.css";

type Member = { id: string; name: string; email: string | null; role: string; roleName: string; status: string; lastLoginAt: string | null; createdAt: string };
type Role = { key: string; name: string; permissions: string[] };

export function OwnerStaff() {
  const qc = useQueryClient();
  const toast = useToast();
  const { user } = useSession();
  const q = useQuery({ queryKey: ["owner", "staff"], queryFn: () => api<{ staff: Member[]; roles: Role[] }>("/owner/staff") });
  const [form, setForm] = useState<{ name: string; email: string; role: string; password: string } | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const reload = () => qc.invalidateQueries({ queryKey: ["owner", "staff"] });

  async function update(m: Member, patch: Partial<Pick<Member, "role" | "status">>) {
    try {
      await api(`/owner/staff/${m.id}`, { method: "PUT", body: { role: patch.role ?? m.role, status: patch.status ?? m.status } });
      toast("Updated.");
      await reload();
    } catch (e) {
      toast(e instanceof ApiError ? (Object.values(e.fields)[0] ?? e.message) : "Please try again.", "error");
    }
  }
  async function create() {
    if (!form) return;
    setErrors({});
    try {
      await api("/owner/staff", { body: form });
      toast(`${form.name} can now sign in. Share the temporary password privately and ask them to change it.`);
      setForm(null);
      await reload();
    } catch (e) {
      if (e instanceof ApiError) setErrors(e.fields);
    }
  }
  const roles = q.data?.roles ?? [];
  return (
    <>
      <PageHead
        title="Staff"
        sub="Who can sign in to this dashboard and what each person can do."
        actions={
          <button className="btn btn-primary btn-sm" onClick={() => setForm({ name: "", email: "", role: "tailor", password: "" })}>
            Add a person
          </button>
        }
      />
      {form ? (
        <section className="panel panel-pad stack-sm" aria-label="Add staff">
          <div className="form-grid cols-2">
            <label className="field">
              <span className="label">Name</span>
              <input className="input" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              {errors.name ? <span className="error">{errors.name}</span> : null}
            </label>
            <label className="field">
              <span className="label">Email</span>
              <input className="input" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
              {errors.email ? <span className="error">{errors.email}</span> : null}
            </label>
            <label className="field">
              <span className="label">Role</span>
              <select className="select" value={form.role} onChange={(e) => setForm({ ...form, role: e.target.value })}>
                {roles.map((r) => (
                  <option key={r.key} value={r.key}>
                    {r.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="field">
              <span className="label">Temporary password</span>
              <input className="input" type="password" autoComplete="new-password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
              <span className="hint">At least 10 characters.</span>
              {errors.password ? <span className="error">{errors.password}</span> : null}
            </label>
          </div>
          <div className="row-wrap">
            <button className="btn btn-primary btn-sm" onClick={create}>
              Add
            </button>
            <button className="btn btn-ghost btn-sm" onClick={() => setForm(null)}>
              Cancel
            </button>
          </div>
        </section>
      ) : null}
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Email</th>
              <th>Role</th>
              <th>Access</th>
              <th>Last sign-in</th>
            </tr>
          </thead>
          <tbody>
            {(q.data?.staff ?? []).map((m) => (
              <tr key={m.id}>
                <td>
                  {m.name}
                  {m.id === user?.id ? <span className="tiny muted"> (you)</span> : null}
                </td>
                <td>{m.email}</td>
                <td>
                  <select className="select" aria-label={`${m.name} role`} value={m.role} disabled={m.id === user?.id} onChange={(e) => update(m, { role: e.target.value })} style={{ width: "auto" }}>
                    {roles.map((r) => (
                      <option key={r.key} value={r.key}>
                        {r.name}
                      </option>
                    ))}
                  </select>
                </td>
                <td>
                  <select className="select" aria-label={`${m.name} access`} value={m.status} disabled={m.id === user?.id} onChange={(e) => update(m, { status: e.target.value })} style={{ width: "auto" }}>
                    <option value="active">Active</option>
                    <option value="disabled">Disabled</option>
                    {m.status === "locked" ? <option value="locked">Locked</option> : null}
                  </select>
                </td>
                <td>{m.lastLoginAt ? formatDateTime(m.lastLoginAt) : "Never"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <section className="stack-sm" aria-labelledby="roles-h">
        <h2 id="roles-h" className={styles.h2}>
          What each role can do
        </h2>
        <dl className={styles.dl}>
          {roles.map((r) => (
            <div key={r.key} style={{ display: "contents" }}>
              <dt>{r.name}</dt>
              <dd className="small">{r.permissions.map((p) => humanize(p.replace(".", " "))).join(", ")}</dd>
            </div>
          ))}
        </dl>
      </section>
    </>
  );
}

type AuditRow = { id: string; actorName: string | null; actorRole: string | null; action: string; objectType: string; objectId: string | null; before: unknown; after: unknown; createdAt: string };

export function OwnerAudit() {
  return (
    <>
      <PageHead title="Audit log" sub="A permanent record of sensitive actions. Entries cannot be edited or deleted." />
      <Suspense>
        <OwnerList<AuditRow>
          endpoint="/owner/audit"
          filters={[
            {
              key: "objectType",
              label: "Area",
              type: "select",
              options: ["order", "payment", "quote", "request", "customer", "user", "upload", "measurement_profile", "asset_manifest", "feature_flag", "business_settings", "content_block"].map((x) => [x, humanize(x)]),
            },
          ]}
          rowKey={(r) => r.id}
          empty="No entries."
          columns={[
            { label: "When", cell: (r) => formatDate(r.createdAt, "short") + " " + new Date(r.createdAt).toISOString().slice(11, 16) },
            { label: "Who", cell: (r) => r.actorName ?? "System" },
            { label: "Action", cell: (r) => humanize(r.action.replaceAll(".", " ")) },
            { label: "On", cell: (r) => `${humanize(r.objectType)}${r.objectId ? ` ${r.objectId.slice(0, 8)}` : ""}` },
            {
              label: "Details",
              cell: (r) =>
                r.before || r.after ? (
                  <details>
                    <summary className="tiny">Show</summary>
                    <pre className="tiny" style={{ whiteSpace: "pre-wrap", maxWidth: 420, margin: 0 }}>
                      {JSON.stringify({ before: r.before, after: r.after }, null, 1)}
                    </pre>
                  </details>
                ) : (
                  ""
                ),
            },
          ]}
        />
      </Suspense>
    </>
  );
}
