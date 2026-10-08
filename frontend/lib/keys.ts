// keyFrom turns a display name into a stable key: hyphens for garment keys (they appear in URLs),
// underscores for option, section and measurement keys. Accents are dropped, so "Boubou brodé"
// becomes "boubou-brode".
export function keyFrom(name: string, sep: "-" | "_"): string {
  const trim = new RegExp(`^${sep}+|${sep}+$`, "g");
  return name
    .toLowerCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^a-z0-9]+/g, sep)
    .replace(trim, "")
    .slice(0, 40)
    .replace(trim, "");
}
