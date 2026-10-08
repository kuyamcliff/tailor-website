#!/usr/bin/env node
// Builds the development stand-in 3D assets for the fitting studio: two dress-form mannequins and
// procedurally generated suit, shirt and dress models with named, switchable parts and morph targets.
//
// These are honest placeholders, generated entirely by this script (no third-party geometry), so the
// studio, option switching, fabric materials and fit logic can be developed and tested end to end.
// They are NOT production garments: replace them with professionally authored, licensed GLB files
// (see docs/3d-assets.md). Every asset is written to the manifest with productionQuality: false.
//
// Usage: node scripts/build-standin-assets.mjs
// Output: public/3d/*.glb (high and low detail), public/3d/manifest.json

import { createHash } from "node:crypto";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const OUT = join(dirname(fileURLToPath(import.meta.url)), "..", "public", "3d");
const TAU = Math.PI * 2;

// ---------------------------------------------------------------------------------------------
// Small math helpers

const lerp = (a, b, t) => a + (b - a) * t;
const clamp = (v, a, b) => Math.min(b, Math.max(a, v));
const smooth = (t) => t * t * (3 - 2 * t);
const gauss = (x, mu, sigma) => Math.exp(-((x - mu) ** 2) / (2 * sigma * sigma));
const spow = (v, p) => Math.sign(v) * Math.abs(v) ** p;

// Interpolates keyframes [[y, ...values]] in y with a centripetal-free Catmull-Rom spline, so
// profiles are smooth through every key (no flat spots that would show as ripples in the shading).
function keyed(keys, y) {
  if (y <= keys[0][0]) return keys[0].slice(1);
  const n = keys.length;
  if (y >= keys[n - 1][0]) return keys[n - 1].slice(1);
  let i = 0;
  while (i < n - 2 && y > keys[i + 1][0]) i++;
  const k0 = keys[Math.max(0, i - 1)];
  const k1 = keys[i];
  const k2 = keys[i + 1];
  const k3 = keys[Math.min(n - 1, i + 2)];
  const t = (y - k1[0]) / (k2[0] - k1[0]);
  const h = k2[0] - k1[0];
  return k1.slice(1).map((_, c) => {
    const p0 = k0[c + 1];
    const p1 = k1[c + 1];
    const p2 = k2[c + 1];
    const p3 = k3[c + 1];
    // Tangents scaled to the segment length (non-uniform knot spacing).
    const m1 = k2[0] === k0[0] ? 0 : ((p2 - p0) / (k2[0] - k0[0])) * h;
    const m2 = k3[0] === k1[0] ? 0 : ((p3 - p1) / (k3[0] - k1[0])) * h;
    const t2 = t * t;
    const t3 = t2 * t;
    return (2 * t3 - 3 * t2 + 1) * p1 + (t3 - 2 * t2 + t) * m1 + (-2 * t3 + 3 * t2) * p2 + (t3 - t2) * m2;
  });
}

const ellipseCirc = (rx, rz) => TAU * Math.sqrt((rx * rx + rz * rz) / 2);

// ---------------------------------------------------------------------------------------------
// Body definitions. Units are metres, Y up, the figure faces +Z. Modifiers (in metres of
// circumference, or of width for shoulders) drive the morph targets shared by bodies and garments.

const BODIES = {
  masculine: {
    height: 1.8,
    torso: [
      [0.8, 0.13, 0.1, 0],
      [0.86, 0.168, 0.12, 0],
      [0.94, 0.178, 0.128, -0.005],
      [1.0, 0.172, 0.122, 0],
      [1.06, 0.158, 0.112, 0.005],
      [1.12, 0.152, 0.108, 0.008],
      [1.2, 0.16, 0.115, 0.012],
      [1.28, 0.172, 0.122, 0.015],
      [1.34, 0.182, 0.122, 0.012],
      [1.4, 0.188, 0.112, 0],
      [1.44, 0.172, 0.095, -0.005],
      [1.47, 0.12, 0.075, -0.005],
      [1.49, 0.068, 0.06, 0],
      [1.5, 0.06, 0.056, 0],
    ],
    neck: { y0: 1.5, y1: 1.6, r: 0.055 },
    bands: { chest: [1.32, 0.07], waist: [1.1, 0.06], hip: [0.96, 0.06], shoulders: [1.41, 0.04] },
    leg: {
      x0: 0.088,
      x1: 0.1,
      keys: [
        [0.06, 0.034],
        [0.15, 0.036],
        [0.3, 0.05],
        [0.42, 0.058],
        [0.5, 0.055],
        [0.55, 0.058],
        [0.7, 0.075],
        [0.84, 0.092],
        [0.92, 0.098],
      ],
    },
    crotch: 0.86,
    arm: {
      x: 0.195,
      y: 1.41,
      z: -0.01,
      len: 0.58,
      splay: 0.18,
      keys: [
        [0, 0.052],
        [0.1, 0.05],
        [0.45, 0.04],
        [0.5, 0.038],
        [0.85, 0.03],
        [1, 0.027],
      ],
    },
    hemJacket: 0.8,
  },
  feminine: {
    height: 1.68,
    torso: [
      [0.76, 0.12, 0.095, 0],
      [0.8, 0.172, 0.122, -0.004],
      [0.86, 0.182, 0.13, -0.008],
      [0.92, 0.172, 0.122, 0],
      [1.0, 0.142, 0.1, 0.006],
      [1.04, 0.13, 0.095, 0.008],
      [1.1, 0.14, 0.1, 0.01],
      [1.17, 0.152, 0.112, 0.02],
      [1.22, 0.158, 0.125, 0.03],
      [1.27, 0.152, 0.11, 0.015],
      [1.32, 0.158, 0.095, 0],
      [1.36, 0.145, 0.085, -0.005],
      [1.38, 0.1, 0.07, -0.004],
      [1.395, 0.058, 0.05, 0],
      [1.4, 0.05, 0.048, 0],
    ],
    neck: { y0: 1.4, y1: 1.49, r: 0.047 },
    bands: { chest: [1.22, 0.06], waist: [1.04, 0.05], hip: [0.88, 0.06], shoulders: [1.32, 0.04] },
    leg: {
      x0: 0.082,
      x1: 0.095,
      keys: [
        [0.05, 0.03],
        [0.12, 0.032],
        [0.26, 0.045],
        [0.38, 0.054],
        [0.46, 0.05],
        [0.5, 0.052],
        [0.66, 0.07],
        [0.78, 0.09],
        [0.86, 0.1],
      ],
    },
    crotch: 0.8,
    arm: {
      x: 0.165,
      y: 1.315,
      z: -0.01,
      len: 0.54,
      splay: 0.2,
      keys: [
        [0, 0.044],
        [0.1, 0.042],
        [0.45, 0.034],
        [0.5, 0.032],
        [0.85, 0.026],
        [1, 0.024],
      ],
    },
    hemJacket: 0.76,
  },
};

const BODY_MORPHS = ["chest", "waist", "hip", "shoulders"];
const MORPH_STEP = { chest: 0.1, waist: 0.1, hip: 0.1, shoulders: 0.05 }; // influence 1 = +10 cm (shoulders +5 cm)

// Torso cross-section at height y with modifiers applied. `clampLow` keeps garments hanging straight
// below the hips instead of following the body into the legs.
function torsoAt(B, y, mods, clampLow = false) {
  let yy = y;
  if (clampLow) yy = Math.max(y, B.crotch + 0.02);
  let [rx, rz, cz] = keyed(B.torso, yy);
  for (const k of ["chest", "waist", "hip"]) {
    const d = mods[k] ?? 0;
    if (!d) continue;
    const [mu, sigma] = B.bands[k];
    const w = gauss(yy, mu, sigma);
    const c = ellipseCirc(rx, rz);
    const f = 1 + (d / c) * w;
    rx *= f;
    rz *= f;
  }
  const sh = mods.shoulders ?? 0;
  if (sh) rx += (sh / 2) * gauss(yy, B.bands.shoulders[0], B.bands.shoulders[1]);
  return { rx, rz, cz };
}

function legAt(B, y, side, mods) {
  const t = clamp((B.crotch + 0.06 - y) / (B.crotch + 0.06), 0, 1);
  const x = side * lerp(B.leg.x0, B.leg.x1, t);
  let [r] = keyed(B.leg.keys, y);
  r += ((0.012 * (mods.hip ?? 0)) / 0.1) * gauss(y, B.crotch, 0.12);
  return { x, r };
}

function armFrame(B, side, mods) {
  const sx = side * (B.arm.x + (mods.shoulders ?? 0) / 2);
  const dir = norm([side * B.arm.splay, -1, 0.02]);
  return { origin: [sx, B.arm.y, B.arm.z], dir };
}

function norm(v) {
  const l = Math.hypot(v[0], v[1], v[2]) || 1;
  return [v[0] / l, v[1] / l, v[2] / l];
}
function cross(a, b) {
  return [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
}

// Point on the torso surface (θ = 0 is the centre front, positive θ toward the wearer's left, +X).
function torsoPoint(B, y, theta, mods, offset = 0, clampLow = false, p = 0.82) {
  const { rx, rz, cz } = torsoAt(B, y, mods, clampLow);
  return [spow(Math.sin(theta), p) * (rx + offset), y, cz + spow(Math.cos(theta), p) * (rz + offset)];
}

// ---------------------------------------------------------------------------------------------
// Mesh building

class Mesh {
  constructor(name, material) {
    this.name = name;
    this.material = material;
    this.positions = [];
    this.uvs = [];
    this.indices = [];
  }
  get vertexCount() {
    return this.positions.length / 3;
  }
}

// grid builds a (nu+1) x (nv+1) vertex grid from fn(u, v) -> [x, y, z]. UVs are in metres of arc
// length (u) and height (v) so fabric textures keep a consistent scale across every part.
function grid(mesh, nu, nv, fn, { closed = false, flip = false } = {}) {
  const base = mesh.vertexCount;
  const rows = [];
  for (let j = 0; j <= nv; j++) {
    const row = [];
    for (let i = 0; i <= nu; i++) row.push(fn(i / nu, j / nv));
    rows.push(row);
  }
  for (let j = 0; j <= nv; j++) {
    let arc = 0;
    for (let i = 0; i <= nu; i++) {
      const p = rows[j][i];
      if (i > 0) {
        const q = rows[j][i - 1];
        arc += Math.hypot(p[0] - q[0], p[1] - q[1], p[2] - q[2]);
      }
      mesh.positions.push(p[0], p[1], p[2]);
      mesh.uvs.push(arc, p[1]);
    }
  }
  const W = nu + 1;
  for (let j = 0; j < nv; j++) {
    for (let i = 0; i < nu; i++) {
      const a = base + j * W + i;
      const b = a + 1;
      const c = a + W;
      const d = c + 1;
      if (flip) mesh.indices.push(a, b, c, b, d, c);
      else mesh.indices.push(a, c, b, b, c, d);
    }
  }
  mesh.seams ??= [];
  if (closed) for (let j = 0; j <= nv; j++) mesh.seams.push([base + j * W, base + j * W + nu]);
}

// Tube along the arm direction, used for arms, sleeves and cuffs.
function armTube(mesh, B, side, mods, s0, s1, radius, nu, nv) {
  const { origin, dir } = armFrame(B, side, mods);
  const a = norm(cross(dir, [0, 0, 1]));
  const b = norm(cross(a, dir));
  grid(
    mesh,
    nu,
    nv,
    (u, v) => {
      const s = lerp(s0, s1, v);
      const c = [
        origin[0] + dir[0] * s * B.arm.len,
        origin[1] + dir[1] * s * B.arm.len,
        origin[2] + dir[2] * s * B.arm.len,
      ];
      const r = radius(s);
      const th = u * TAU;
      const ca = Math.cos(th) * r;
      const sb = Math.sin(th) * r * 1.08;
      return [c[0] + a[0] * ca + b[0] * sb, c[1] + a[1] * ca + b[1] * sb, c[2] + a[2] * ca + b[2] * sb];
    },
    { closed: true, flip: side < 0 },
  );
}

// Ellipsoid, used for the hands, feet and the neck cap.
function ellipsoid(mesh, c, r, nu, nv, rotate) {
  grid(
    mesh,
    nu,
    nv,
    (u, v) => {
      const th = u * TAU;
      const ph = v * Math.PI;
      let p = [Math.sin(ph) * Math.cos(th) * r[0], -Math.cos(ph) * r[1], Math.sin(ph) * Math.sin(th) * r[2]];
      if (rotate) p = rotate(p);
      return [c[0] + p[0], c[1] + p[1], c[2] + p[2]];
    },
    { closed: true, flip: true },
  );
}

// Disc-like button lying on a surface at point p with outward normal n.
function button(mesh, p, n, r = 0.0105, h = 0.004, segs = 12) {
  const t = norm(cross(n, [0, 1, 0]));
  const bt = cross(n, t);
  grid(
    mesh,
    segs,
    3,
    (u, v) => {
      const th = u * TAU;
      const rr = v < 0.34 ? r * (v / 0.34) : r;
      const hh = v < 0.34 ? h : v < 0.67 ? h : 0;
      const k = v < 0.67 ? 1 : 1;
      return [
        p[0] + n[0] * hh + (t[0] * Math.cos(th) + bt[0] * Math.sin(th)) * rr * k,
        p[1] + n[1] * hh + (t[1] * Math.cos(th) + bt[1] * Math.sin(th)) * rr * k,
        p[2] + n[2] * hh + (t[2] * Math.cos(th) + bt[2] * Math.sin(th)) * rr * k,
      ];
    },
    { closed: true, flip: true },
  );
}

function torsoNormal(B, y, theta, mods, clampLow) {
  const e = 0.002;
  const p = torsoPoint(B, y, theta, mods, 0, clampLow);
  const pu = torsoPoint(B, y, theta + e, mods, 0, clampLow);
  const pv = torsoPoint(B, y + e, theta, mods, 0, clampLow);
  const du = [pu[0] - p[0], pu[1] - p[1], pu[2] - p[2]];
  const dv = [pv[0] - p[0], pv[1] - p[1], pv[2] - p[2]];
  return norm(cross(du, dv));
}

// ---------------------------------------------------------------------------------------------
// Parts. Each builder receives (B, mods, q) where q is the detail level and returns meshes.
// Every part is regenerated with modified parameters to compute its morph target deltas, so all
// parts must produce the same vertex count for any modifiers.

function bodyParts(B, mods, q, extra = {}) {
  const skin = new Mesh("mannequin", "mannequin");
  grid(skin, 48 * q, 40 * q, (u, v) => torsoPoint(B, lerp(B.torso[0][0], B.neck.y0, v), u * TAU, mods), {
    closed: true,
  });
  grid(
    skin,
    24 * q,
    6 * q,
    (u, v) => {
      const y = lerp(B.neck.y0 - 0.005, B.neck.y1, v);
      const th = u * TAU;
      return [Math.sin(th) * B.neck.r, y, Math.cos(th) * B.neck.r * 1.02];
    },
    { closed: true },
  );
  ellipsoid(skin, [0, B.neck.y1, 0], [B.neck.r, 0.018, B.neck.r * 1.02], 24 * q, 6 * q);
  for (const side of [-1, 1]) {
    grid(
      skin,
      24 * q,
      36 * q,
      (u, v) => {
        const y = lerp(B.crotch + 0.07, 0.07, v);
        const { x, r } = legAt(B, y, side, mods);
        const th = u * TAU;
        return [x + Math.sin(th) * r, y, Math.cos(th) * r * 1.05];
      },
      { closed: true, flip: true },
    );
    const footX = side * B.leg.x1;
    ellipsoid(skin, [footX, 0.04, 0.035], [0.042, 0.04, 0.115], 16 * q, 8 * q);
    armTube(skin, B, side, mods, 0, 1, (s) => keyed(B.arm.keys, s)[0], 20 * q, 28 * q);
    const { origin, dir } = armFrame(B, side, mods);
    const wrist = [origin[0] + dir[0] * B.arm.len, origin[1] + dir[1] * B.arm.len, origin[2] + dir[2] * B.arm.len];
    ellipsoid(
      skin,
      [wrist[0] + dir[0] * 0.075, wrist[1] + dir[1] * 0.075, wrist[2]],
      [0.024, 0.08, 0.042],
      14 * q,
      10 * q,
    );
  }
  // The pelvis cap closing the torso between the legs.
  ellipsoid(skin, [0, B.torso[0][0] + 0.005, 0], [B.torso[0][1], 0.04, B.torso[0][2]], 32 * q, 6 * q);
  void extra;
  return [skin];
}

// V-shaped front opening of a jacket: 0 below the top button, widening to the collar.
function jacketOpening(B, y, btnY) {
  const top = B.neck.y0 - 0.03;
  if (y >= btnY) return 0.55 * smooth(clamp((y - btnY) / (top - btnY), 0, 1)) + 0.02;
  return 0.02 + 0.22 * smooth(clamp((btnY - 0.12 - y) / 0.2, 0, 1)) * 0;
}

function jacketParts(B, mods, q) {
  const out = [];
  const btnY = B === BODIES.masculine ? 1.07 : 1.0;
  const off = (y) => 0.016 + 0.008 * smooth(clamp((1.0 - y) / 0.2, 0, 1));
  const body = new Mesh("jacket_body", "fabric");
  grid(body, 56 * q, 36 * q, (u, v) => {
    const y = lerp(B.hemJacket, B.neck.y0 - 0.02, v);
    const open = jacketOpening(B, y, btnY);
    const th = lerp(open, TAU - open, u);
    return torsoPoint(B, y, th, mods, off(y), true);
  });
  out.push(body);

  const collar = new Mesh("jacket_collar", "fabric");
  grid(collar, 24 * q, 4 * q, (u, v) => {
    const th = lerp(0.6, TAU - 0.6, u);
    const y = lerp(B.neck.y0 - 0.035, B.neck.y0 + 0.035, v);
    const r = B.neck.r + 0.02 + 0.03 * (1 - v);
    return [Math.sin(th) * r, y, Math.cos(th) * r * 0.95 - 0.004];
  });
  out.push(collar);

  for (const side of [-1, 1]) {
    const sleeve = new Mesh(side < 0 ? "sleeve_right" : "sleeve_left", "fabric");
    armTube(sleeve, B, side, mods, -0.02, 0.95, (s) => keyed(B.arm.keys, s)[0] + 0.018, 20 * q, 22 * q);
    out.push(sleeve);
  }

  // Lapels lie on the jacket front either side of the opening. width(y) gives each style its outline.
  const lapelStyles = {
    lapel_notch: (t) =>
      t < 0.78 ? lerp(0.08, 0.3, smooth(t / 0.78)) : t < 0.84 ? 0.1 : lerp(0.22, 0.16, (t - 0.84) / 0.16),
    lapel_peak: (t) =>
      t < 0.74
        ? lerp(0.08, 0.32, smooth(t / 0.74))
        : t < 0.84
          ? lerp(0.32, 0.46, (t - 0.74) / 0.1)
          : lerp(0.16, 0.14, (t - 0.84) / 0.16),
    lapel_shawl: (t) => lerp(0.07, 0.26, Math.sin(Math.min(1, t * 1.15) * Math.PI * 0.5)),
  };
  for (const [name, width] of Object.entries(lapelStyles)) {
    const m = new Mesh(name, "fabric");
    m.morphExtras = { lapel_width: (mm) => ({ ...mm, __lapel: 1.35 }) };
    for (const side of [-1, 1]) {
      grid(
        m,
        6 * q,
        18 * q,
        (u, v) => {
          const y = lerp(btnY, B.neck.y0 - 0.025, v);
          const open = jacketOpening(B, y, btnY);
          const w = width(v) * (mods.__lapel ?? 1);
          const th = open + u * w;
          return torsoPoint(B, y, side > 0 ? th : -th, mods, off(y) + 0.004 + 0.003 * (1 - u), true);
        },
        { flip: side < 0 },
      );
    }
    out.push(m);
  }

  const btns = { buttons_1: [btnY], buttons_2: [btnY, btnY - 0.1], buttons_3: [btnY + 0.09, btnY, btnY - 0.09] };
  for (const [name, ys] of Object.entries(btns)) {
    const m = new Mesh(name, "button");
    for (const y of ys) {
      const p = torsoPoint(B, y, 0.035, mods, off(y) + 0.002, true);
      button(m, p, torsoNormal(B, y, 0.035, mods, true), 0.0105, 0.004, 10 * q);
    }
    out.push(m);
  }
  const db = new Mesh("buttons_db", "button");
  for (const y of [btnY + 0.06, btnY - 0.02, btnY - 0.1])
    for (const th of [-0.32, 0.32]) {
      const p = torsoPoint(B, y, th, mods, off(y) + 0.002, true);
      button(db, p, torsoNormal(B, y, th, mods, true), 0.0105, 0.004, 10 * q);
    }
  out.push(db);

  const pocketY = B.hemJacket + 0.13;
  const pockets = { pocket_flap: [0.055, 0.004], pocket_jetted: [0.014, 0.0025], pocket_patch: [0.16, 0.004] };
  for (const [name, [h, lift]] of Object.entries(pockets)) {
    const m = new Mesh(name, "fabric");
    for (const side of [-1, 1]) {
      grid(
        m,
        8 * q,
        3,
        (u, v) => {
          const th = side * lerp(0.62, 1.32, u);
          const y = name === "pocket_patch" ? pocketY + 0.03 - v * h : pocketY - v * h;
          return torsoPoint(B, y, th, mods, off(y) + lift, true);
        },
        { flip: side < 0 },
      );
    }
    out.push(m);
  }
  const chest = new Mesh("chest_pocket", "fabric");
  grid(chest, 6 * q, 2, (u, v) => {
    const y = (B === BODIES.masculine ? 1.3 : 1.2) - v * 0.022;
    return torsoPoint(B, y, lerp(0.55, 0.95, u), mods, off(y) + 0.003, true);
  });
  out.push(chest);
  return out;
}

function trouserParts(B, mods, q) {
  const out = [];
  const waistY = B === BODIES.masculine ? 1.08 : 1.02;
  const t = new Mesh("trousers", "fabric");
  grid(t, 48 * q, 10 * q, (u, v) => torsoPoint(B, lerp(B.crotch - 0.02, waistY, v), u * TAU, mods, 0.012), {
    closed: true,
  });
  const hemY = 0.035;
  const legR = (y) => {
    const { r } = legAt(B, y, 1, mods);
    const knee = B.crotch * 0.6;
    const tapered = Math.max(r + 0.02, lerp(0.07, 0.085, clamp((y - hemY) / (knee - hemY), 0, 1)));
    const wide = lerp(0.11, r + 0.025, clamp((y - hemY) / (B.crotch - hemY), 0, 1) ** 1.6);
    return lerp(tapered, Math.max(tapered, wide), mods.__legWide ?? 0);
  };
  for (const side of [-1, 1]) {
    grid(
      t,
      24 * q,
      30 * q,
      (u, v) => {
        const y = lerp(B.crotch + 0.04, hemY, v);
        const { x } = legAt(B, y, side, mods);
        const r = legR(y);
        const th = u * TAU;
        return [x + Math.sin(th) * r, y, Math.cos(th) * r * 1.04];
      },
      { closed: true, flip: true },
    );
  }
  t.morphExtras = { leg_wide: (m) => ({ ...m, __legWide: 1 }) };
  out.push(t);
  const band = new Mesh("waistband", "fabric");
  grid(band, 48 * q, 2, (u, v) => torsoPoint(B, lerp(waistY - 0.035, waistY + 0.002, v), u * TAU, mods, 0.016), {
    closed: true,
  });
  out.push(band);
  const cuff = new Mesh("trouser_cuff", "fabric");
  for (const side of [-1, 1]) {
    grid(
      cuff,
      24 * q,
      2,
      (u, v) => {
        const y = lerp(hemY, hemY + 0.04, v);
        const { x } = legAt(B, y, side, mods);
        const r = legR(y) + 0.004;
        const th = u * TAU;
        return [x + Math.sin(th) * r, y, Math.cos(th) * r * 1.04];
      },
      { closed: true, flip: true },
    );
  }
  cuff.morphExtras = { leg_wide: (m) => ({ ...m, __legWide: 1 }) };
  out.push(cuff);
  return out;
}

function shirtCollar(B, mods, q, name, style) {
  const m = new Mesh(name, "shirt");
  const ny = B.neck.y0;
  const stand = style === "mandarin" ? 0.045 : 0.035;
  grid(m, 28 * q, 3, (u, v) => {
    const th = lerp(0.12, TAU - 0.12, u);
    const r = B.neck.r + 0.01;
    return [Math.sin(th) * r, lerp(ny - 0.01, ny + stand, v), Math.cos(th) * r * 1.02];
  });
  if (style !== "mandarin") {
    const spread = style === "spread" ? 0.95 : 0.6;
    const len = style === "spread" ? 0.06 : 0.075;
    for (const side of [-1, 1]) {
      grid(
        m,
        6 * q,
        4,
        (u, v) => {
          const y = lerp(ny + stand * 0.9, ny - len, v);
          const th = side * lerp(0.1 + v * 0.05, 0.1 + lerp(0.5, spread, v), u);
          return torsoPoint(B, Math.min(y, ny - 0.002), th, mods, 0.011 + 0.01 * (1 - v), false);
        },
        { flip: side < 0 },
      );
    }
    if (style === "button_down") {
      for (const side of [-1, 1]) {
        const y = ny - len + 0.012;
        const th = side * 0.55;
        button(m, torsoPoint(B, y, th, mods, 0.014), torsoNormal(B, y, th, mods), 0.004, 0.002, 8);
      }
    }
  }
  return m;
}

function shirtParts(B, mods, q, { base = false } = {}) {
  const out = [];
  const body = new Mesh(base ? "shirt_base" : "shirt_body", "shirt");
  grid(
    body,
    48 * q,
    30 * q,
    (u, v) => torsoPoint(B, lerp(base ? 1.0 : B.hemJacket - 0.02, B.neck.y0 - 0.005, v), u * TAU, mods, 0.007, true),
    { closed: true },
  );
  out.push(body);
  if (base) {
    out.push(Object.assign(shirtCollar(B, mods, q, "shirt_base_collar", "spread"), {}));
    return out;
  }
  const placket = new Mesh("shirt_buttons", "button_light");
  for (let i = 0; i < 6; i++) {
    const y = B.neck.y0 - 0.06 - i * 0.095;
    button(placket, torsoPoint(B, y, 0, mods, 0.0085, true), torsoNormal(B, y, 0, mods, true), 0.0055, 0.002, 8);
  }
  out.push(placket);
  out.push(shirtCollar(B, mods, q, "collar_spread", "spread"));
  out.push(shirtCollar(B, mods, q, "collar_button_down", "button_down"));
  out.push(shirtCollar(B, mods, q, "collar_mandarin", "mandarin"));
  const long = new Mesh("sleeve_long", "shirt");
  const short = new Mesh("sleeve_short", "shirt");
  const barrel = new Mesh("cuff_barrel", "shirt");
  const french = new Mesh("cuff_french", "shirt");
  for (const side of [-1, 1]) {
    armTube(long, B, side, mods, -0.02, 0.9, (s) => keyed(B.arm.keys, s)[0] + 0.012, 18 * q, 20 * q);
    armTube(short, B, side, mods, -0.02, 0.36, (s) => keyed(B.arm.keys, s)[0] + 0.016 + 0.006 * s, 18 * q, 6 * q);
    armTube(barrel, B, side, mods, 0.86, 0.95, () => keyed(B.arm.keys, 0.9)[0] + 0.014, 18 * q, 2);
    armTube(french, B, side, mods, 0.84, 0.96, () => keyed(B.arm.keys, 0.9)[0] + 0.019, 18 * q, 2);
  }
  out.push(long, short, barrel, french);
  const pocket = new Mesh("shirt_pocket", "shirt");
  grid(pocket, 6 * q, 4, (u, v) => {
    const y = (B === BODIES.masculine ? 1.3 : 1.2) - v * 0.12;
    return torsoPoint(B, y, lerp(0.38, 0.85, u), mods, 0.0105, true);
  });
  out.push(pocket);
  return out;
}

function dressParts(B, mods, q) {
  const out = [];
  const waistY = 1.04;
  const top = B.neck.y0 - 0.005;
  const under = 1.3;
  const bodice = new Mesh("bodice", "fabric");
  grid(bodice, 56 * q, 18 * q, (u, v) => torsoPoint(B, lerp(waistY - 0.01, under, v), u * TAU, mods, 0.006), {
    closed: true,
  });
  out.push(bodice);
  // Neckline pieces cover the upper bodice and shoulders; the top edge follows each cut.
  const cuts = {
    neckline_round: (th) => 0.03 + 0.05 * gauss(th, 0, 0.55),
    neckline_v: (th) => 0.03 + 0.16 * Math.max(0, 1 - Math.abs(th) / 0.55),
    neckline_sweetheart: (th) =>
      0.03 + 0.11 * Math.max(0, 1 - Math.abs(th) / 0.75) - 0.04 * gauss(Math.abs(th), 0.32, 0.12),
  };
  for (const [name, cut] of Object.entries(cuts)) {
    const m = new Mesh(name, "fabric");
    grid(
      m,
      56 * q,
      8 * q,
      (u, v) => {
        const th = u * TAU;
        const signed = th > Math.PI ? th - TAU : th;
        const yTop = top - (Math.abs(signed) < Math.PI / 2 ? cut(signed) : 0.025);
        return torsoPoint(B, lerp(under - 0.01, yTop, v), th, mods, 0.006);
      },
      { closed: true },
    );
    out.push(m);
  }
  const none = new Mesh("sleeve_none", "fabric");
  out.push(none);
  const cap = new Mesh("sleeve_cap", "fabric");
  const long = new Mesh("sleeve_long", "fabric");
  for (const side of [-1, 1]) {
    armTube(
      cap,
      B,
      side,
      mods,
      -0.03,
      0.17,
      (s) => keyed(B.arm.keys, s)[0] + 0.012 + 0.03 * clamp(s / 0.17, 0, 1),
      18 * q,
      4 * q,
    );
    armTube(long, B, side, mods, -0.03, 0.97, (s) => keyed(B.arm.keys, s)[0] + 0.01, 18 * q, 22 * q);
  }
  out.push(cap, long);

  // Skirts: hem height and flare come from the length morphs (knee by default).
  const hemFor = (m) => lerp(lerp(0.5, 0.3, m.__midi ?? 0), 0.03, m.__floor ?? 0);
  const shapes = {
    skirt_a_line: (t, hem) => lerp(0.2, lerp(0.34, 0.46, (0.5 - hem) / 0.47), t),
    skirt_pencil: (t, hem) => lerp(0.2, 0.15 + 0.02 * ((0.5 - hem) / 0.47), smooth(t)),
    skirt_ballgown: (t, hem) => lerp(0.22, lerp(0.5, 0.72, (0.5 - hem) / 0.47), Math.sqrt(t)),
  };
  for (const [name, shape] of Object.entries(shapes)) {
    const m = new Mesh(name, "fabric");
    grid(
      m,
      64 * q,
      26 * q,
      (u, v) => {
        const hem = hemFor(mods);
        const hipY = B.bands.hip[0];
        const y = lerp(waistY + 0.005, hem, v);
        const th = u * TAU;
        const body = torsoAt(B, Math.max(y, hipY), mods, true);
        const t = clamp((hipY - y) / (hipY - hem), 0, 1);
        const flare = y > hipY ? 0 : shape(t, hem);
        const rx = Math.max(body.rx + 0.012, flare);
        const rz = Math.max(body.rz + 0.012, flare * 0.82);
        return [
          spow(Math.sin(th), 0.85) * rx,
          y,
          body.cz * (1 - t) + spow(Math.cos(th), 0.85) * rz - (name === "skirt_ballgown" ? 0.03 * t : 0),
        ];
      },
      { closed: true },
    );
    m.morphExtras = { length_midi: (mm) => ({ ...mm, __midi: 1 }), length_floor: (mm) => ({ ...mm, __floor: 1 }) };
    out.push(m);
  }
  return out;
}

// ---------------------------------------------------------------------------------------------
// Assembly: generate the base meshes, then each morph target as the vertex difference.

function build(builder, B, q) {
  const base = builder(B, {}, q);
  for (let i = 0; i < base.length; i++) {
    const mesh = base[i];
    mesh.targets = [];
    const names = [...BODY_MORPHS, ...Object.keys(mesh.morphExtras ?? {})];
    for (const name of names) {
      const mods = BODY_MORPHS.includes(name) ? { [name]: MORPH_STEP[name] } : mesh.morphExtras[name]({});
      const moved = builder(B, mods, q)[i];
      if (moved.positions.length !== mesh.positions.length)
        throw new Error(`morph ${name} changed vertex count of ${mesh.name}`);
      mesh.targets.push({ name, delta: moved.positions.map((p, k) => p - mesh.positions[k]) });
    }
  }
  return base;
}

function computeNormals(mesh) {
  const n = new Float32Array(mesh.positions.length);
  const P = mesh.positions;
  for (let i = 0; i < mesh.indices.length; i += 3) {
    const [a, b, c] = [mesh.indices[i] * 3, mesh.indices[i + 1] * 3, mesh.indices[i + 2] * 3];
    const e1 = [P[b] - P[a], P[b + 1] - P[a + 1], P[b + 2] - P[a + 2]];
    const e2 = [P[c] - P[a], P[c + 1] - P[a + 1], P[c + 2] - P[a + 2]];
    const f = cross(e1, e2);
    for (const v of [a, b, c]) {
      n[v] += f[0];
      n[v + 1] += f[1];
      n[v + 2] += f[2];
    }
  }
  for (const [a, b] of mesh.seams ?? []) {
    for (let k = 0; k < 3; k++) {
      const s = n[a * 3 + k] + n[b * 3 + k];
      n[a * 3 + k] = s;
      n[b * 3 + k] = s;
    }
  }
  for (let i = 0; i < n.length; i += 3) {
    const l = Math.hypot(n[i], n[i + 1], n[i + 2]) || 1;
    n[i] /= l;
    n[i + 1] /= l;
    n[i + 2] /= l;
  }
  return n;
}

// ---------------------------------------------------------------------------------------------
// Minimal GLB writer (glTF 2.0): positions, normals, UVs, indices and morph targets per mesh.

const MATERIALS = {
  mannequin: { baseColorFactor: [0.62, 0.58, 0.53, 1], roughnessFactor: 0.85, metallicFactor: 0 },
  fabric: { baseColorFactor: [0.35, 0.36, 0.4, 1], roughnessFactor: 0.82, metallicFactor: 0, doubleSided: true },
  shirt: { baseColorFactor: [0.9, 0.9, 0.88, 1], roughnessFactor: 0.75, metallicFactor: 0, doubleSided: true },
  button: { baseColorFactor: [0.07, 0.06, 0.055, 1], roughnessFactor: 0.35, metallicFactor: 0 },
  button_light: { baseColorFactor: [0.85, 0.84, 0.8, 1], roughnessFactor: 0.3, metallicFactor: 0 },
};

function writeGLB(meshes, rootName) {
  const chunks = [];
  let offset = 0;
  const bufferViews = [];
  const accessors = [];
  const add = (typed, target, accessor) => {
    const bytes = Buffer.from(typed.buffer, typed.byteOffset, typed.byteLength);
    const pad = (4 - (bytes.length % 4)) % 4;
    chunks.push(bytes, Buffer.alloc(pad));
    bufferViews.push({ buffer: 0, byteOffset: offset, byteLength: bytes.length, ...(target ? { target } : {}) });
    offset += bytes.length + pad;
    accessors.push({ bufferView: bufferViews.length - 1, ...accessor });
    return accessors.length - 1;
  };
  const minmax = (arr) => {
    const min = [Infinity, Infinity, Infinity];
    const max = [-Infinity, -Infinity, -Infinity];
    for (let i = 0; i < arr.length; i += 3)
      for (let k = 0; k < 3; k++) {
        min[k] = Math.min(min[k], arr[i + k]);
        max[k] = Math.max(max[k], arr[i + k]);
      }
    return { min, max };
  };
  const matNames = Object.keys(MATERIALS);
  const gltfMeshes = [];
  const nodes = [{ name: rootName, children: [] }];
  for (const m of meshes) {
    if (!m.vertexCount) {
      nodes.push({ name: m.name });
      nodes[0].children.push(nodes.length - 1);
      continue;
    }
    const pos = new Float32Array(m.positions);
    const nor = computeNormals(m);
    const uv = new Float32Array(m.uvs);
    const idx = m.vertexCount > 65535 ? new Uint32Array(m.indices) : new Uint16Array(m.indices);
    const prim = {
      attributes: {
        POSITION: add(pos, 34962, { componentType: 5126, count: m.vertexCount, type: "VEC3", ...minmax(pos) }),
        NORMAL: add(nor, 34962, { componentType: 5126, count: m.vertexCount, type: "VEC3" }),
        TEXCOORD_0: add(uv, 34962, { componentType: 5126, count: m.vertexCount, type: "VEC2" }),
      },
      indices: add(idx, 34963, {
        componentType: idx instanceof Uint32Array ? 5125 : 5123,
        count: idx.length,
        type: "SCALAR",
      }),
      material: matNames.indexOf(m.material),
      // Morph targets use sparse accessors: only vertices that actually move are stored.
      targets: m.targets.map((t) => {
        const moved = [];
        for (let v = 0; v < m.vertexCount; v++) {
          if (Math.abs(t.delta[v * 3]) + Math.abs(t.delta[v * 3 + 1]) + Math.abs(t.delta[v * 3 + 2]) > 5e-5)
            moved.push(v); // ignore sub-0.05 mm movement
        }
        const values = new Float32Array(Math.max(1, moved.length) * 3);
        moved.forEach((v, i) => values.set(t.delta.slice(v * 3, v * 3 + 3), i * 3));
        const indices = new Uint32Array(moved.length ? moved : [0]);
        const range = minmax(values);
        const vIdx = add(indices, null, { componentType: 5125, count: indices.length, type: "SCALAR" });
        const vVal = add(values, null, { componentType: 5126, count: indices.length, type: "VEC3" });
        // The sparse data lives in its own buffer views; drop the helper accessors created by add().
        const idxView = accessors[vIdx].bufferView;
        const valView = accessors[vVal].bufferView;
        accessors.splice(vIdx, 2);
        accessors.push({
          componentType: 5126,
          count: m.vertexCount,
          type: "VEC3",
          min: range.min.map((x) => Math.min(x, 0)),
          max: range.max.map((x) => Math.max(x, 0)),
          sparse: {
            count: indices.length,
            indices: { bufferView: idxView, componentType: 5125 },
            values: { bufferView: valView },
          },
        });
        return { POSITION: accessors.length - 1 };
      }),
    };
    gltfMeshes.push({
      name: m.name,
      primitives: [prim],
      weights: m.targets.map(() => 0),
      extras: { targetNames: m.targets.map((t) => t.name) },
    });
    nodes.push({ name: m.name, mesh: gltfMeshes.length - 1 });
    nodes[0].children.push(nodes.length - 1);
  }
  const gltf = {
    asset: { version: "2.0", generator: "atelier build-standin-assets.mjs" },
    scene: 0,
    scenes: [{ nodes: [0] }],
    nodes,
    meshes: gltfMeshes,
    materials: matNames.map((name) => {
      const { doubleSided, ...pbr } = MATERIALS[name];
      return { name, pbrMetallicRoughness: pbr, ...(doubleSided ? { doubleSided: true } : {}) };
    }),
    buffers: [{ byteLength: offset }],
    bufferViews,
    accessors,
  };
  const bin = Buffer.concat(chunks);
  let json = Buffer.from(JSON.stringify(gltf));
  json = Buffer.concat([json, Buffer.alloc((4 - (json.length % 4)) % 4, 0x20)]);
  const header = Buffer.alloc(12);
  const total = 12 + 8 + json.length + 8 + bin.length;
  header.writeUInt32LE(0x46546c67, 0);
  header.writeUInt32LE(2, 4);
  header.writeUInt32LE(total, 8);
  const jh = Buffer.alloc(8);
  jh.writeUInt32LE(json.length, 0);
  jh.writeUInt32LE(0x4e4f534a, 4);
  const bh = Buffer.alloc(8);
  bh.writeUInt32LE(bin.length, 0);
  bh.writeUInt32LE(0x004e4942, 4);
  return Buffer.concat([header, jh, json, bh, bin]);
}

// ---------------------------------------------------------------------------------------------

const MODELS = [
  { file: "body-masculine", kind: "body", body: "masculine", builder: (B, m, q) => bodyParts(B, m, q) },
  { file: "body-feminine", kind: "body", body: "feminine", builder: (B, m, q) => bodyParts(B, m, q) },
  {
    file: "suit",
    kind: "garment",
    body: "masculine",
    builder: (B, m, q) => [...shirtParts(B, m, q, { base: true }), ...trouserParts(B, m, q), ...jacketParts(B, m, q)],
  },
  {
    file: "shirt",
    kind: "garment",
    body: "masculine",
    builder: (B, m, q) => [...trouserParts(B, m, q), ...shirtParts(B, m, q)],
  },
  { file: "dress", kind: "garment", body: "feminine", builder: (B, m, q) => dressParts(B, m, q) },
];

const jacketOnly = ["trousers", "waistband", "trouser_cuff"];
const trousersOnly = [
  "jacket_body",
  "jacket_collar",
  "sleeve_left",
  "sleeve_right",
  "lapel_notch",
  "lapel_peak",
  "lapel_shawl",
  "buttons_1",
  "buttons_2",
  "buttons_3",
  "buttons_db",
  "pocket_flap",
  "pocket_jetted",
  "pocket_patch",
  "chest_pocket",
  "shirt_base",
  "shirt_base_collar",
];

const ENTRIES = [
  { assetKey: "body-masculine", file: "body-masculine", kind: "body", garmentTypeKeys: [], bodyModel: "masculine" },
  { assetKey: "body-feminine", file: "body-feminine", kind: "body", garmentTypeKeys: [], bodyModel: "feminine" },
  {
    assetKey: "suit-standin",
    file: "suit",
    kind: "garment",
    garmentTypeKeys: ["suit"],
    bodyModel: "masculine",
    baseHidden: [],
  },
  {
    assetKey: "jacket-standin",
    file: "suit",
    kind: "garment",
    garmentTypeKeys: ["jacket"],
    bodyModel: "masculine",
    baseHidden: jacketOnly,
  },
  {
    assetKey: "trousers-standin",
    file: "suit",
    kind: "garment",
    garmentTypeKeys: ["trousers"],
    bodyModel: "masculine",
    baseHidden: trousersOnly,
  },
  {
    assetKey: "shirt-standin",
    file: "shirt",
    kind: "garment",
    garmentTypeKeys: ["shirt"],
    bodyModel: "masculine",
    baseHidden: [],
  },
  {
    assetKey: "dress-standin",
    file: "dress",
    kind: "garment",
    garmentTypeKeys: ["dress"],
    bodyModel: "feminine",
    baseHidden: [],
  },
  {
    assetKey: "gown-standin",
    file: "dress",
    kind: "garment",
    garmentTypeKeys: ["gown"],
    bodyModel: "feminine",
    baseHidden: [],
  },
];

// The mannequin's own measurements, so the studio can turn a customer's numbers into morph amounts.
function baseMeasurements(B) {
  const circ = (y) => {
    const { rx, rz } = torsoAt(B, y, {});
    let len = 0;
    let prev = torsoPoint(B, y, 0, {});
    for (let i = 1; i <= 360; i++) {
      const p = torsoPoint(B, y, (i / 360) * TAU, {});
      len += Math.hypot(p[0] - prev[0], p[2] - prev[2]);
      prev = p;
    }
    void rx;
    void rz;
    return Math.round(len * 1000);
  };
  return {
    height: Math.round(B.height * 1000),
    chest: circ(B.bands.chest[0]),
    waist: circ(B.bands.waist[0]),
    hip: circ(B.bands.hip[0]),
    shoulder: Math.round((2 * B.arm.x + 0.04) * 1000),
  };
}

mkdirSync(OUT, { recursive: true });
const files = {};
for (const model of MODELS) {
  const B = BODIES[model.body];
  files[model.file] = { parts: [], morphs: new Set(), lods: [] };
  for (const [lod, q] of [
    ["high", 1.5],
    ["low", 1],
  ]) {
    const meshes = build(model.builder, B, q);
    const glb = writeGLB(meshes, model.file);
    const name = lod === "high" ? `${model.file}.glb` : `${model.file}-low.glb`;
    writeFileSync(join(OUT, name), glb);
    files[model.file].lods.push({
      lod,
      url: `/3d/${name}`,
      bytes: glb.length,
      sha256: createHash("sha256").update(glb).digest("hex"),
    });
    if (lod === "high") {
      files[model.file].parts = meshes.map((m) => m.name);
      for (const m of meshes) for (const t of m.targets ?? []) files[model.file].morphs.add(t.name);
    }
    console.log(
      `${name}: ${(glb.length / 1024).toFixed(0)} KB, ${meshes.reduce((n, m) => n + m.vertexCount, 0)} vertices`,
    );
  }
}

const manifest = {
  generatedBy: "frontend/scripts/build-standin-assets.mjs",
  note: "Development stand-ins generated procedurally. Not production garments. Replace with licensed, professionally authored assets before launch.",
  assets: ENTRIES.map((e) => ({
    assetKey: e.assetKey,
    version: 1,
    kind: e.kind,
    garmentTypeKeys: e.garmentTypeKeys,
    files: files[e.file].lods,
    bodyCompat: {
      bodyModel: e.bodyModel,
      heightM: BODIES[e.bodyModel].height,
      morphStepM: MORPH_STEP,
      baseMm: baseMeasurements(BODIES[e.bodyModel]),
    },
    supportedOptions: {
      parts: files[e.file].parts,
      morphs: [...files[e.file].morphs],
      baseHidden: e.baseHidden ?? [],
      bodyModel: e.bodyModel,
    },
    textureSetVersion: "ambientcg-cc0-1",
    license: {
      source: "Generated by frontend/scripts/build-standin-assets.mjs",
      license: "Project-owned",
      author: "Project",
    },
    productionQuality: false,
    notes: "Procedural development stand-in. Shapes are simplified and do not show real drape or construction.",
  })),
};
writeFileSync(join(OUT, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
console.log(`manifest: ${manifest.assets.length} assets`);
