// Quality management for the fitting studio. The starting level comes from what the device reports
// (memory, cores, pointer, screen, renderer), and FrameBudget steps it down or up from measured frame
// times while the scene is moving. Nothing here touches three.js, so it is unit tested directly.

export type Quality = "ultra" | "high" | "medium" | "low";

// Camera distance limits in metres, shared by the viewer and the zoom buttons.
export const ZOOM = { min: 1.6, max: 5, initial: 3.3, step: 0.45 };

export const qualityOrder: Quality[] = ["low", "medium", "high", "ultra"];

export const qualityLabel: Record<Quality, string> = {
  ultra: "Highest",
  high: "High",
  medium: "Medium",
  low: "Light",
};

export type QualitySettings = {
  dpr: [number, number]; // device pixel ratio range
  interactionDpr: number; // pixel ratio while the customer is turning or zooming
  lod: "high" | "low"; // which model file to load
  shadows: boolean;
  shadowResolution: number;
  anisotropy: number;
  normalMaps: boolean;
  rimLight: boolean;
};

export const qualitySettings: Record<Quality, QualitySettings> = {
  ultra: {
    dpr: [1, 2],
    interactionDpr: 1.25,
    lod: "high",
    shadows: true,
    shadowResolution: 1024,
    anisotropy: 8,
    normalMaps: true,
    rimLight: true,
  },
  high: {
    dpr: [1, 1.75],
    interactionDpr: 1,
    lod: "high",
    shadows: true,
    shadowResolution: 512,
    anisotropy: 4,
    normalMaps: true,
    rimLight: true,
  },
  medium: {
    dpr: [1, 1.5],
    interactionDpr: 1,
    lod: "low",
    shadows: true,
    shadowResolution: 256,
    anisotropy: 2,
    normalMaps: true,
    rimLight: false,
  },
  low: {
    dpr: [0.75, 1],
    interactionDpr: 0.75,
    lod: "low",
    shadows: false,
    shadowResolution: 128,
    anisotropy: 1,
    normalMaps: false,
    rimLight: false,
  },
};

export type DeviceInfo = {
  deviceMemory?: number; // GB, Chrome only
  cores?: number;
  coarsePointer: boolean;
  screenWidth: number;
  pixelRatio: number;
  renderer?: string; // unmasked GPU renderer string, when the browser exposes it
  maxTextureSize?: number;
  saveData?: boolean;
};

const softwareRenderer = /swiftshader|llvmpipe|software|basic render|mesa offscreen/i;
const weakGpu = /mali-[gt]?[0-9]{1,2}\b|adreno \(tm\) [3-5][0-9]{2}|powervr|intel\(r\) (hd|uhd) graphics [0-9]{3}\b/i;

// startingQuality picks the highest level the device is likely to sustain. FrameBudget then corrects
// it from measured frame times, so this errs on the cautious side.
export function startingQuality(d: DeviceInfo): Quality {
  if (d.saveData) return "low";
  if (d.renderer && softwareRenderer.test(d.renderer)) return "low";
  if (d.maxTextureSize !== undefined && d.maxTextureSize < 4096) return "low";
  if (d.deviceMemory !== undefined && d.deviceMemory <= 2) return "low";
  if (d.cores !== undefined && d.cores <= 2) return "low";
  let q: Quality = "ultra";
  const cap = (to: Quality) => {
    if (qualityOrder.indexOf(to) < qualityOrder.indexOf(q)) q = to;
  };
  if (d.renderer && weakGpu.test(d.renderer)) cap("medium");
  if (d.deviceMemory !== undefined && d.deviceMemory <= 4) cap("medium");
  if (d.cores !== undefined && d.cores <= 4) cap("medium");
  if (d.coarsePointer) cap("high");
  if (d.screenWidth < 768) cap("medium");
  return q;
}

export function stepQuality(q: Quality, dir: "up" | "down", ceiling: Quality = "ultra"): Quality {
  const i = qualityOrder.indexOf(q) + (dir === "up" ? 1 : -1);
  const max = qualityOrder.indexOf(ceiling);
  return qualityOrder[Math.max(0, Math.min(max, i))] ?? q;
}

// FrameBudget watches frame times in windows of consecutive frames. With on-demand rendering the
// canvas is idle most of the time, so gaps longer than maxGapMs end a run instead of counting as slow
// frames. A slow window steps quality down at once; it only steps back up after several fast windows,
// and never more than maxChanges times, so the scene does not flicker between levels.
export class FrameBudget {
  private samples: number[] = [];
  private fastWindows = 0;
  private changes = 0;

  constructor(
    private readonly opts = {
      windowSize: 40,
      slowMs: 30,
      fastMs: 13,
      fastWindowsToStepUp: 4,
      maxGapMs: 250,
      maxChanges: 4,
    },
  ) {}

  // push records one frame time and returns a step to take, if any.
  push(frameMs: number): "up" | "down" | null {
    if (this.changes >= this.opts.maxChanges) return null;
    if (frameMs <= 0 || frameMs > this.opts.maxGapMs) {
      this.samples = [];
      return null;
    }
    this.samples.push(frameMs);
    if (this.samples.length < this.opts.windowSize) return null;
    const sorted = [...this.samples].sort((a, b) => a - b);
    this.samples = [];
    // The median ignores single hitches such as a texture upload.
    const median = sorted[Math.floor(sorted.length / 2)] ?? 0;
    if (median > this.opts.slowMs) {
      this.fastWindows = 0;
      this.changes++;
      return "down";
    }
    if (median < this.opts.fastMs) {
      this.fastWindows++;
      if (this.fastWindows >= this.opts.fastWindowsToStepUp) {
        this.fastWindows = 0;
        this.changes++;
        return "up";
      }
    } else {
      this.fastWindows = 0;
    }
    return null;
  }
}

export function readDeviceInfo(): DeviceInfo {
  const nav = navigator as Navigator & {
    deviceMemory?: number;
    connection?: { saveData?: boolean };
  };
  const info: DeviceInfo = {
    deviceMemory: nav.deviceMemory,
    cores: nav.hardwareConcurrency || undefined,
    coarsePointer: window.matchMedia("(pointer: coarse)").matches,
    screenWidth: Math.min(window.screen?.width ?? window.innerWidth, window.innerWidth),
    pixelRatio: window.devicePixelRatio || 1,
    saveData: nav.connection?.saveData,
  };
  try {
    const gl =
      document.createElement("canvas").getContext("webgl2") ?? document.createElement("canvas").getContext("webgl");
    if (gl) {
      info.maxTextureSize = gl.getParameter(gl.MAX_TEXTURE_SIZE) as number;
      const dbg = gl.getExtension("WEBGL_debug_renderer_info");
      if (dbg) info.renderer = String(gl.getParameter(dbg.UNMASKED_RENDERER_WEBGL));
      gl.getExtension("WEBGL_lose_context")?.loseContext();
    }
  } catch {
    // Capability probing is best effort.
  }
  return info;
}
