import { describe, expect, it } from "vitest";
import { FrameBudget, startingQuality, stepQuality, type DeviceInfo } from "@/features/studio/quality";

const desktop: DeviceInfo = {
  deviceMemory: 16,
  cores: 12,
  coarsePointer: false,
  screenWidth: 1440,
  pixelRatio: 2,
  renderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060)",
  maxTextureSize: 16384,
};

describe("startingQuality", () => {
  it("gives a capable desktop the highest level", () => {
    expect(startingQuality(desktop)).toBe("ultra");
  });
  it("starts phones lower", () => {
    expect(startingQuality({ ...desktop, coarsePointer: true, screenWidth: 390 })).toBe("medium");
    expect(startingQuality({ ...desktop, coarsePointer: true, screenWidth: 820 })).toBe("high");
  });
  it("uses the light level for weak or software rendering", () => {
    expect(startingQuality({ ...desktop, renderer: "Google SwiftShader" })).toBe("low");
    expect(startingQuality({ ...desktop, deviceMemory: 2 })).toBe("low");
    expect(startingQuality({ ...desktop, cores: 2 })).toBe("low");
    expect(startingQuality({ ...desktop, saveData: true })).toBe("low");
    expect(startingQuality({ ...desktop, maxTextureSize: 2048 })).toBe("low");
  });
  it("caps budget GPUs and small memory at medium", () => {
    expect(startingQuality({ ...desktop, renderer: "Mali-G52" })).toBe("medium");
    expect(startingQuality({ ...desktop, deviceMemory: 4 })).toBe("medium");
  });
});

describe("stepQuality", () => {
  it("moves one level and stops at the ends", () => {
    expect(stepQuality("high", "down")).toBe("medium");
    expect(stepQuality("low", "down")).toBe("low");
    expect(stepQuality("ultra", "up")).toBe("ultra");
    expect(stepQuality("medium", "up", "medium")).toBe("medium");
  });
});

describe("FrameBudget", () => {
  const feed = (b: FrameBudget, ms: number, n: number) => {
    const out: string[] = [];
    for (let i = 0; i < n; i++) {
      const r = b.push(ms);
      if (r) out.push(r);
    }
    return out;
  };

  it("steps down after one slow window", () => {
    const b = new FrameBudget();
    expect(feed(b, 45, 39)).toEqual([]);
    expect(feed(b, 45, 1)).toEqual(["down"]);
  });

  it("needs several fast windows to step up", () => {
    const b = new FrameBudget();
    expect(feed(b, 8, 40 * 3)).toEqual([]);
    expect(feed(b, 8, 40)).toEqual(["up"]);
  });

  it("ignores idle gaps from on-demand rendering", () => {
    const b = new FrameBudget();
    for (let i = 0; i < 200; i++) {
      expect(b.push(16)).toBeNull();
      expect(b.push(5000)).toBeNull(); // the canvas was idle
    }
  });

  it("ignores single hitches", () => {
    const b = new FrameBudget();
    const out: string[] = [];
    for (let i = 0; i < 400; i++) {
      const r = b.push(i % 20 === 0 ? 120 : 16);
      if (r) out.push(r);
    }
    expect(out).toEqual([]);
  });

  it("stops changing after a few steps so the view does not flicker", () => {
    const b = new FrameBudget();
    const out = [...feed(b, 45, 400), ...feed(b, 8, 4000)];
    expect(out.length).toBe(4);
  });
});
