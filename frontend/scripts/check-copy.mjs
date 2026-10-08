#!/usr/bin/env node
// Copy checks for customer-facing text. Fails (exit 1) on:
//  - em dashes (U+2014) and spaced en dashes used as sentence dashes
//  - placeholder text such as lorem ipsum
//  - template marketing phrases that read as generated filler
// Scans the frontend source and the backend's default content.
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(fileURLToPath(import.meta.url), "..", "..", "..");
const targets = ["frontend/app", "frontend/features", "frontend/components", "frontend/lib", "backend/internal/seed", "backend/internal/notifications"];
const exts = new Set([".ts", ".tsx", ".css", ".go", ".json", ".md"]);
const rules = [
  { re: /—/, why: "em dash" },
  { re: / – /, why: "spaced en dash used as a dash" },
  { re: /lorem ipsum/i, why: "placeholder text" },
  { re: /\b(unlock your|elevate your|revolutionary|cutting-edge|world-class|seamless experience|game[- ]changer|AI[- ]powered)\b/i, why: "template marketing phrase" },
];

const problems = [];
function walk(dir) {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name.startsWith(".")) continue;
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) walk(p);
    else if (exts.has(name.slice(name.lastIndexOf(".")))) {
      if (name.endsWith("_test.go") || name.includes(".test.")) continue;
      readFileSync(p, "utf8")
        .split("\n")
        .forEach((line, i) => {
          for (const r of rules) if (r.re.test(line)) problems.push(`${relative(root, p)}:${i + 1}: ${r.why}: ${line.trim().slice(0, 120)}`);
        });
    }
  }
}
for (const t of targets) walk(join(root, t));
if (problems.length) {
  console.error(`Copy check failed (${problems.length}):\n` + problems.join("\n"));
  process.exit(1);
}
console.log("Copy check passed.");
