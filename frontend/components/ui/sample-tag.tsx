// SampleTag labels licensed stock photography seeded for development, so it is never mistaken for the atelier's own work.
export function SampleTag({ show, position = "top" }: { show?: boolean; position?: "top" | "bottom" }) {
  if (!show) return null;
  return (
    <span className={`sample-tag ${position === "bottom" ? "sample-tag-bottom" : ""}`} title="Licensed stock photo used until the atelier's own photography is added">
      Sample photo
    </span>
  );
}
