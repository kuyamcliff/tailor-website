#!/usr/bin/env node
// Copies the Draco and Basis (KTX2) decoders that ship with three.js into public/3d/decoders/<revision>/,
// so the studio can load compressed models and textures from our own origin (the CSP allows no CDN).
// The revision in the path keeps the immutable cache on /3d/ correct when three.js is upgraded.
// Runs before dev and build; the output is not committed.
import { cpSync, existsSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const three = join(root, "node_modules", "three");
const { version } = JSON.parse(readFileSync(join(three, "package.json"), "utf8"));
const revision = version.split(".")[1];
const out = join(root, "public", "3d", "decoders");
const target = join(out, revision);

if (!existsSync(join(target, "basis", "basis_transcoder.wasm"))) {
  rmSync(out, { recursive: true, force: true });
  mkdirSync(target, { recursive: true });
  cpSync(join(three, "examples", "jsm", "libs", "draco", "gltf"), join(target, "draco"), { recursive: true });
  cpSync(join(three, "examples", "jsm", "libs", "basis"), join(target, "basis"), {
    recursive: true,
    filter: (src) => !src.endsWith(".md"),
  });
  console.log(`3D decoders copied for three r${revision}`);
}
