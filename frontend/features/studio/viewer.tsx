"use client";

import { Suspense, useEffect, useMemo, useRef } from "react";
import { Canvas, useFrame, useThree } from "@react-three/fiber";
import { ContactShadows, OrbitControls, useGLTF } from "@react-three/drei";
import * as THREE from "three";
import type { Pbr } from "@/lib/types";

export type ViewAngle = "front" | "45" | "side" | "back";
export type Quality = "high" | "low";

export type ViewerProps = {
  bodyUrl: string | null;
  garmentUrl: string | null;
  visibility: Record<string, boolean>;
  morphs: Record<string, number>;
  heightScale: number;
  fabric: { pbr: Pbr; colorHex: string } | null;
  view: ViewAngle;
  turn: number; // extra rotation in radians from the rotate buttons
  quality: Quality;
  reducedMotion: boolean;
  onReady?: () => void;
  label: string;
};

const azimuth: Record<ViewAngle, number> = { front: 0, "45": Math.PI / 4, side: Math.PI / 2, back: Math.PI };

export function Viewer(props: ViewerProps) {
  return (
    <Canvas
      className="studio-canvas"
      aria-label={props.label}
      role="img"
      dpr={props.quality === "low" ? [1, 1.25] : [1, 1.75]}
      gl={{
        antialias: props.quality === "high",
        powerPreference: props.quality === "low" ? "low-power" : "high-performance",
        preserveDrawingBuffer: true,
      }}
      camera={{ position: [0, 1.1, 3.3], fov: 33, near: 0.1, far: 30 }}
      onCreated={({ gl }) => {
        gl.toneMapping = THREE.ACESFilmicToneMapping;
        gl.toneMappingExposure = 1.25;
        gl.outputColorSpace = THREE.SRGBColorSpace;
      }}
    >
      <color attach="background" args={["#121214"]} />
      <hemisphereLight args={["#f4ece0", "#1b1a19", 0.9]} />
      <directionalLight position={[2.5, 4, 3]} intensity={2.1} color="#fff6ea" />
      <directionalLight position={[-3, 2.5, -2]} intensity={0.9} color="#c9d4e6" />
      <directionalLight position={[0, 1.5, -4]} intensity={0.6} />
      <Suspense fallback={null}>
        <Scene {...props} />
      </Suspense>
      <ContactShadows
        position={[0, 0, 0]}
        opacity={0.55}
        scale={3}
        blur={2.4}
        far={1.2}
        resolution={props.quality === "low" ? 256 : 512}
      />
      <CameraRig view={props.view} turn={props.turn} reducedMotion={props.reducedMotion} />
    </Canvas>
  );
}

function CameraRig({ view, turn, reducedMotion }: { view: ViewAngle; turn: number; reducedMotion: boolean }) {
  const controls = useRef<React.ComponentRef<typeof OrbitControls>>(null);
  const goal = useRef<number | null>(null);
  const { camera } = useThree();
  useEffect(() => {
    goal.current = azimuth[view] + turn;
  }, [view, turn]);
  useFrame((_, dt) => {
    const c = controls.current;
    if (!c || goal.current === null) return;
    const current = c.getAzimuthalAngle();
    let diff = goal.current - current;
    diff = Math.atan2(Math.sin(diff), Math.cos(diff));
    if (Math.abs(diff) < 0.002 || reducedMotion) {
      c.setAzimuthalAngle(current + diff);
      goal.current = null;
    } else {
      c.setAzimuthalAngle(current + diff * Math.min(1, dt * 7));
    }
    c.update();
  });
  useEffect(() => {
    camera.lookAt(0, 0.98, 0);
  }, [camera]);
  return (
    <OrbitControls
      ref={controls}
      target={[0, 0.98, 0]}
      enablePan={false}
      minDistance={1.6}
      maxDistance={5}
      minPolarAngle={Math.PI * 0.2}
      maxPolarAngle={Math.PI * 0.58}
      enableDamping={!reducedMotion}
      onStart={() => {
        goal.current = null;
      }}
    />
  );
}

function Scene({ bodyUrl, garmentUrl, visibility, morphs, heightScale, fabric, onReady }: ViewerProps) {
  const fabricMaterial = useFabricMaterial(fabric);
  useEffect(() => {
    onReady?.();
  }, [onReady, bodyUrl, garmentUrl]);
  return (
    <group scale={[1, heightScale, 1]}>
      {bodyUrl ? <Model url={bodyUrl} visibility={{}} morphs={morphs} fabricMaterial={null} /> : null}
      {garmentUrl ? (
        <Model url={garmentUrl} visibility={visibility} morphs={morphs} fabricMaterial={fabricMaterial} />
      ) : null}
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
  const { scene } = useGLTF(url, false);
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
  }, [root, originals, visibility, morphs, fabricMaterial]);

  return <primitive object={root} />;
}

const loader = new THREE.TextureLoader();

function useFabricMaterial(fabric: ViewerProps["fabric"]) {
  const key = fabric ? JSON.stringify(fabric) : "";
  const material = useMemo(() => {
    if (!fabric) return null;
    const { pbr, colorHex } = fabric;
    const repeat = pbr.repeat ?? 6;
    const tex = (url: string | undefined, srgb: boolean) => {
      if (!url) return null;
      const t = loader.load(url);
      t.wrapS = t.wrapT = THREE.RepeatWrapping;
      t.repeat.set(repeat, repeat);
      t.anisotropy = 4;
      if (srgb) t.colorSpace = THREE.SRGBColorSpace;
      return t;
    };
    const m = new THREE.MeshPhysicalMaterial({
      name: "fabric",
      color: new THREE.Color(pbr.tint ? colorHex : "#ffffff"),
      map: tex(pbr.colorMap, true),
      normalMap: tex(pbr.normalMap, false),
      roughnessMap: tex(pbr.roughnessMap, false),
      roughness: pbr.roughness ?? 0.85,
      metalness: 0,
      sheen: pbr.sheen ?? 0,
      sheenRoughness: 0.8,
      sheenColor: new THREE.Color(pbr.tint ? colorHex : "#ffffff").lerp(new THREE.Color("#ffffff"), 0.4),
      clearcoat: pbr.clearcoat ?? 0,
      side: THREE.DoubleSide,
    });
    m.normalScale.set(0.6, 0.6);
    return m;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed by the serialised fabric
  }, [key]);
  useEffect(
    () => () => {
      if (!material) return;
      for (const t of [material.map, material.normalMap, material.roughnessMap]) t?.dispose();
      material.dispose();
    },
    [material],
  );
  return material;
}

export function preloadModel(url: string) {
  useGLTF.preload(url, false);
}

export function webglAvailable(): boolean {
  try {
    const c = document.createElement("canvas");
    return Boolean(c.getContext("webgl2") || c.getContext("webgl"));
  } catch {
    return false;
  }
}
