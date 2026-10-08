"use client";

import { Suspense, useEffect, useMemo, useRef } from "react";
import { Canvas, useFrame, useThree } from "@react-three/fiber";
import { ContactShadows, Html, OrbitControls, useGLTF } from "@react-three/drei";
import * as THREE from "three";
import type { Pbr } from "@/lib/types";
import { dracoPath, gltfExtensions, ktx2Loader, releaseModel, retainModel } from "./loaders";
import { FrameBudget, qualitySettings, ZOOM, type Quality } from "./quality";

export type ViewAngle = "front" | "45" | "side" | "135" | "back" | "right";

export type FitMarker = { zone: string; label: string; state: string; anchor: [number, number, number] };

export type RenderStats = {
  fps: number;
  frameMs: number;
  calls: number;
  triangles: number;
  geometries: number;
  textures: number;
};

export type ViewerProps = {
  bodyUrl: string | null;
  garmentUrl: string | null;
  visibility: Record<string, boolean>;
  morphs: Record<string, number>;
  heightScale: number;
  fabric: { pbr: Pbr; colorHex: string } | null;
  view: ViewAngle;
  turn: number; // extra rotation in radians from the turn buttons and keys
  zoom: number; // camera distance in metres
  resetKey: number; // changes when the customer asks for a reset
  autoRotate: boolean;
  quality: Quality;
  antialias: boolean; // fixed for the life of the canvas
  reducedMotion: boolean;
  fitMarkers: FitMarker[] | null;
  label: string;
  onQualityStep?: (dir: "up" | "down") => void;
  onZoomChange?: (distance: number) => void;
  onTextureError?: () => void;
  onStats?: (s: RenderStats) => void;
  onReset?: () => void;
};

export const azimuth: Record<ViewAngle, number> = {
  front: 0,
  "45": Math.PI / 4,
  side: Math.PI / 2,
  "135": (3 * Math.PI) / 4,
  back: Math.PI,
  right: -Math.PI / 2,
};
const TARGET = new THREE.Vector3(0, 0.98, 0);
const POLAR = Math.acos((1.1 - 0.98) / Math.hypot(3.3, 1.1 - 0.98));

export function Viewer(props: ViewerProps) {
  const s = qualitySettings[props.quality];
  return (
    <Canvas
      className="studio-canvas"
      aria-label={props.label}
      role="img"
      frameloop="demand"
      dpr={s.dpr}
      gl={{
        antialias: props.antialias,
        powerPreference: props.quality === "low" ? "low-power" : "high-performance",
        preserveDrawingBuffer: true,
      }}
      camera={{ position: [0, 1.1, ZOOM.initial], fov: 33, near: 0.1, far: 30 }}
      onCreated={({ gl }) => {
        gl.toneMapping = THREE.ACESFilmicToneMapping;
        gl.toneMappingExposure = 1.25;
        gl.outputColorSpace = THREE.SRGBColorSpace;
      }}
      onDoubleClick={() => props.onReset?.()}
    >
      <color attach="background" args={["#121214"]} />
      <hemisphereLight args={["#f4ece0", "#1b1a19", 0.9]} />
      <directionalLight position={[2.5, 4, 3]} intensity={2.1} color="#fff6ea" />
      <directionalLight position={[-3, 2.5, -2]} intensity={0.9} color="#c9d4e6" />
      {s.rimLight ? <directionalLight position={[0, 1.5, -4]} intensity={0.6} /> : null}
      <Suspense fallback={null}>
        <Scene {...props} />
      </Suspense>
      {s.shadows ? (
        <ContactShadows
          key={s.shadowResolution}
          position={[0, 0, 0]}
          opacity={0.55}
          scale={3}
          blur={2.4}
          far={1.2}
          resolution={s.shadowResolution}
        />
      ) : null}
      <CameraRig {...props} />
      <Budget quality={props.quality} onStep={props.onQualityStep} onStats={props.onStats} />
    </Canvas>
  );
}

// CameraRig eases the camera toward the requested angle and distance, keeps rendering only while
// something moves, and lowers the pixel ratio while the customer drags or pinches.
function CameraRig({ view, turn, zoom, resetKey, autoRotate, reducedMotion, quality, onZoomChange }: ViewerProps) {
  const controls = useRef<React.ComponentRef<typeof OrbitControls>>(null);
  const goal = useRef<{ azimuth: number | null; distance: number | null; polar: number | null }>({
    azimuth: null,
    distance: null,
    polar: null,
  });
  const { camera, invalidate, setDpr } = useThree();
  const s = qualitySettings[quality];

  useEffect(() => {
    goal.current.azimuth = azimuth[view] + turn;
    invalidate();
  }, [view, turn, invalidate]);
  useEffect(() => {
    goal.current.distance = zoom;
    invalidate();
  }, [zoom, invalidate]);
  useEffect(() => {
    if (!resetKey) return;
    goal.current = { azimuth: azimuth.front, distance: ZOOM.initial, polar: POLAR };
    invalidate();
  }, [resetKey, invalidate]);
  useEffect(() => {
    camera.lookAt(TARGET);
  }, [camera]);

  useFrame((_, dt) => {
    const c = controls.current;
    if (!c) return;
    const g = goal.current;
    const k = reducedMotion ? 1 : Math.min(1, dt * 7);
    let moving = false;
    if (g.azimuth !== null) {
      const current = c.getAzimuthalAngle();
      const diff = Math.atan2(Math.sin(g.azimuth - current), Math.cos(g.azimuth - current));
      if (Math.abs(diff) < 0.002) {
        c.setAzimuthalAngle(current + diff);
        g.azimuth = null;
      } else {
        c.setAzimuthalAngle(current + diff * k);
        moving = true;
      }
    }
    if (g.polar !== null) {
      const current = c.getPolarAngle();
      const diff = g.polar - current;
      if (Math.abs(diff) < 0.002) {
        c.setPolarAngle(g.polar);
        g.polar = null;
      } else {
        c.setPolarAngle(current + diff * k);
        moving = true;
      }
    }
    if (g.distance !== null) {
      const current = c.getDistance();
      const target = THREE.MathUtils.clamp(g.distance, ZOOM.min, ZOOM.max);
      const diff = target - current;
      const next = Math.abs(diff) < 0.002 ? target : current + diff * k;
      if (next === target) g.distance = null;
      else moving = true;
      const dir = camera.position.clone().sub(c.target).normalize();
      camera.position.copy(c.target).addScaledVector(dir, next);
    }
    c.update();
    if (moving || autoRotate) invalidate();
  });

  return (
    <OrbitControls
      ref={controls}
      target={TARGET}
      enablePan={false}
      minDistance={ZOOM.min}
      maxDistance={ZOOM.max}
      minPolarAngle={Math.PI * 0.2}
      maxPolarAngle={Math.PI * 0.58}
      enableDamping={!reducedMotion}
      autoRotate={autoRotate}
      autoRotateSpeed={reducedMotion ? 0.6 : 1.2}
      onStart={() => {
        goal.current = { azimuth: null, distance: null, polar: null };
        setDpr(s.interactionDpr);
      }}
      onEnd={() => {
        setDpr(s.dpr);
        const c = controls.current;
        if (c) onZoomChange?.(c.getDistance());
        invalidate();
      }}
    />
  );
}

// Budget measures frame times while the scene is animating and asks for a quality change when the
// device cannot keep up, or has room to spare. It also reports renderer counts for diagnostics.
function Budget({
  quality,
  onStep,
  onStats,
}: {
  quality: Quality;
  onStep?: (d: "up" | "down") => void;
  onStats?: (s: RenderStats) => void;
}) {
  const budget = useMemo(() => new FrameBudget(), []);
  const last = useRef(0);
  const frames = useRef({ n: 0, ms: 0 });
  const gl = useThree((s) => s.gl);
  useEffect(() => {
    last.current = 0;
  }, [quality]);
  useFrame(() => {
    const now = performance.now();
    const dt = last.current ? now - last.current : 0;
    last.current = now;
    const step = budget.push(dt);
    if (step) onStep?.(step);
    if (dt > 0 && dt < 250) {
      frames.current.n++;
      frames.current.ms += dt;
    }
  });
  // Reported on a timer: with on-demand rendering there may be no frames while the scene is still.
  useEffect(() => {
    if (!onStats) return;
    const id = setInterval(() => {
      const f = frames.current;
      onStats({
        fps: f.n ? Math.round(1000 / (f.ms / f.n)) : 0,
        frameMs: f.n ? Math.round((f.ms / f.n) * 10) / 10 : 0,
        calls: gl.info.render.calls,
        triangles: gl.info.render.triangles,
        geometries: gl.info.memory.geometries,
        textures: gl.info.memory.textures,
      });
      frames.current = { n: 0, ms: 0 };
    }, 500);
    return () => clearInterval(id);
  }, [gl, onStats]);
  return null;
}

function Scene(props: ViewerProps) {
  const { bodyUrl, garmentUrl, visibility, morphs, heightScale, fabric, quality, fitMarkers, onTextureError } = props;
  const fabricMaterial = useFabricMaterial(fabric, quality, onTextureError);
  return (
    <group scale={[1, heightScale, 1]}>
      {bodyUrl ? <Model url={bodyUrl} visibility={{}} morphs={morphs} fabricMaterial={null} /> : null}
      {garmentUrl ? (
        <Model url={garmentUrl} visibility={visibility} morphs={morphs} fabricMaterial={fabricMaterial} />
      ) : null}
      {fitMarkers?.map((m) => (
        <FitPin key={m.zone} marker={m} />
      ))}
    </group>
  );
}

function Model({
  url,
  visibility,
  morphs,
  fabricMaterial,
}: {
  url: string;
  visibility: Record<string, boolean>;
  morphs: Record<string, number>;
  fabricMaterial: THREE.Material | null;
}) {
  const gl = useThree((s) => s.gl);
  const invalidate = useThree((s) => s.invalidate);
  const { scene } = useGLTF(url, dracoPath, true, gltfExtensions(gl));
  useEffect(() => {
    retainModel(url, scene);
    return () => releaseModel(url);
  }, [url, scene]);
  const root = useMemo(() => scene.clone(true), [scene]);
  const originals = useMemo(() => {
    const m = new Map<THREE.Mesh, THREE.Material>();
    root.traverse((o) => {
      if ((o as THREE.Mesh).isMesh) m.set(o as THREE.Mesh, (o as THREE.Mesh).material as THREE.Material);
    });
    return m;
  }, [root]);

  useEffect(() => {
    root.traverse((o) => {
      const mesh = o as THREE.Mesh;
      if (!mesh.isMesh) return;
      mesh.visible = visibility[mesh.name] ?? true;
      const orig = originals.get(mesh)!;
      mesh.material = fabricMaterial && orig.name === "fabric" ? fabricMaterial : orig;
      if (mesh.morphTargetDictionary && mesh.morphTargetInfluences) {
        for (const [name, idx] of Object.entries(mesh.morphTargetDictionary))
          mesh.morphTargetInfluences[idx] = morphs[name] ?? 0;
      }
    });
    invalidate();
  }, [root, originals, visibility, morphs, fabricMaterial, invalidate]);

  return <primitive object={root} />;
}

// FitPin labels a fit zone on the figure. It fades out when the point faces away from the camera, so
// labels on the far side do not float over the front.
function FitPin({ marker }: { marker: FitMarker }) {
  const el = useRef<HTMLDivElement>(null);
  const camera = useThree((s) => s.camera);
  const normal = useMemo(() => {
    const [x, , z] = marker.anchor;
    const n = new THREE.Vector3(x, 0, z);
    return n.lengthSq() > 0 ? n.normalize() : new THREE.Vector3(0, 0, 1);
  }, [marker.anchor]);
  useFrame(() => {
    if (!el.current) return;
    const toCam = new THREE.Vector3(camera.position.x, 0, camera.position.z).normalize();
    el.current.style.opacity = normal.dot(toCam) > -0.15 ? "1" : "0";
  });
  return (
    <Html position={marker.anchor} center zIndexRange={[20, 0]} wrapperClass="fit-pin-wrap">
      <div ref={el} className="fit-pin" data-state={marker.state} aria-hidden>
        <span className="fit-pin-dot" />
        <span className="fit-pin-label">{marker.label}</span>
      </div>
    </Html>
  );
}

const textureLoader = new THREE.TextureLoader();

function useFabricMaterial(fabric: ViewerProps["fabric"], quality: Quality, onError?: () => void) {
  const gl = useThree((s) => s.gl);
  const invalidate = useThree((s) => s.invalidate);
  const s = qualitySettings[quality];
  const key = fabric ? JSON.stringify(fabric) : "";
  const material = useMemo(() => {
    if (!fabric) return null;
    const { pbr, colorHex } = fabric;
    const repeat = pbr.repeat ?? 6;
    const m = new THREE.MeshPhysicalMaterial({
      name: "fabric",
      color: new THREE.Color(pbr.tint ? colorHex : "#ffffff"),
      roughness: pbr.roughness ?? 0.85,
      metalness: 0,
      sheen: pbr.sheen ?? 0,
      sheenRoughness: 0.8,
      sheenColor: new THREE.Color(pbr.tint ? colorHex : "#ffffff").lerp(new THREE.Color("#ffffff"), 0.4),
      clearcoat: pbr.clearcoat ?? 0,
      side: THREE.DoubleSide,
    });
    m.normalScale.set(0.6, 0.6);
    let failed = false;
    const tex = (url: string | undefined, slot: "map" | "normalMap" | "roughnessMap", srgb: boolean) => {
      if (!url) return;
      const done = (t: THREE.Texture) => {
        if (m.userData.disposed) {
          t.dispose();
          return;
        }
        t.wrapS = t.wrapT = THREE.RepeatWrapping;
        t.repeat.set(repeat, repeat);
        t.anisotropy = Math.min(s.anisotropy, gl.capabilities.getMaxAnisotropy());
        if (srgb) t.colorSpace = THREE.SRGBColorSpace;
        m[slot] = t;
        m.needsUpdate = true;
        invalidate();
      };
      // A texture that fails leaves the flat fabric colour in place and says so once.
      const fail = () => {
        if (slot === "map" && !pbr.tint) m.color.set(colorHex);
        m.needsUpdate = true;
        invalidate();
        if (!failed) {
          failed = true;
          onError?.();
        }
      };
      if (/\.ktx2(\?|$)/i.test(url)) ktx2Loader(gl).load(url, done, undefined, fail);
      else textureLoader.load(url, done, undefined, fail);
    };
    tex(pbr.colorMap, "map", true);
    if (s.normalMaps) tex(pbr.normalMap, "normalMap", false);
    tex(pbr.roughnessMap, "roughnessMap", false);
    return m;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed by the serialised fabric and quality
  }, [key, s.normalMaps, s.anisotropy]);
  useEffect(
    () => () => {
      if (!material) return;
      material.userData.disposed = true;
      for (const t of [material.map, material.normalMap, material.roughnessMap]) t?.dispose();
      material.dispose();
    },
    [material],
  );
  return material;
}

export function webglAvailable(): boolean {
  try {
    const c = document.createElement("canvas");
    return Boolean(c.getContext("webgl2") || c.getContext("webgl"));
  } catch {
    return false;
  }
}
