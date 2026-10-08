import { REVISION, type Material, type Mesh, type Object3D, type Texture, type WebGLRenderer } from "three";
import { KTX2Loader } from "three/examples/jsm/loaders/KTX2Loader.js";
import { useGLTF } from "@react-three/drei";

// Loader setup for the studio: Draco, Meshopt and KTX2 (Basis) decoding from our own origin, and a
// small least-recently-used cache so switching garments back and forth does not download again, while
// models that drop out of the cache release their GPU memory.

const decoders = `/3d/decoders/${REVISION}`;
export const dracoPath = `${decoders}/draco/`;

let ktx2: KTX2Loader | null = null;

// ktx2Loader returns one shared KTX2 loader configured for this renderer's texture formats.
export function ktx2Loader(gl: WebGLRenderer): KTX2Loader {
  if (!ktx2) {
    ktx2 = new KTX2Loader().setTranscoderPath(`${decoders}/basis/`).detectSupport(gl);
  }
  return ktx2;
}

type ExtendLoader = NonNullable<Parameters<typeof useGLTF>[3]>;

// extendLoader is passed to useGLTF so models with KHR_texture_basisu textures can load. drei's
// GLTFLoader comes from three-stdlib, whose KTX2Loader type differs only nominally from three's.
export function gltfExtensions(gl: WebGLRenderer): ExtendLoader {
  return (loader) => {
    loader.setKTX2Loader(ktx2Loader(gl) as unknown as Parameters<typeof loader.setKTX2Loader>[0]);
  };
}

const KEEP = 4; // models kept after they stop being shown (two garments with their bodies)
const inUse = new Map<string, number>();
const idle: string[] = []; // least recently released first
const loaded = new Map<string, Object3D>();

export function retainModel(url: string, scene: Object3D) {
  loaded.set(url, scene);
  inUse.set(url, (inUse.get(url) ?? 0) + 1);
  const i = idle.indexOf(url);
  if (i >= 0) idle.splice(i, 1);
}

export function releaseModel(url: string) {
  const n = (inUse.get(url) ?? 1) - 1;
  if (n > 0) {
    inUse.set(url, n);
    return;
  }
  inUse.delete(url);
  idle.push(url);
  while (idle.length > KEEP) {
    const evict = idle.shift()!;
    const scene = loaded.get(evict);
    loaded.delete(evict);
    if (scene) disposeObject(scene);
    useGLTF.clear(evict);
  }
}

export function disposeObject(root: Object3D) {
  const textures = new Set<Texture>();
  const materials = new Set<Material>();
  root.traverse((o) => {
    const mesh = o as Mesh;
    if (!mesh.isMesh) return;
    mesh.geometry?.dispose();
    for (const m of Array.isArray(mesh.material) ? mesh.material : [mesh.material]) if (m) materials.add(m);
  });
  for (const m of materials) {
    for (const v of Object.values(m)) if (v && (v as Texture).isTexture) textures.add(v as Texture);
    m.dispose();
  }
  for (const t of textures) t.dispose();
}

// Counts for tests and the diagnostics overlay.
export function modelCacheState() {
  return { inUse: [...inUse.keys()], idle: [...idle] };
}
