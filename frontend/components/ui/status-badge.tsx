import { toneOf, humanize } from "@/lib/format";

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const tone = toneOf(status);
  return (
    <span className={`badge ${tone ? `badge-${tone}` : ""}`}>
      <span className="dot" aria-hidden />
      {label ?? humanize(status)}
    </span>
  );
}
