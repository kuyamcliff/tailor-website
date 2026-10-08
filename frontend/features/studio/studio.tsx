"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Component, useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Maximize,
  Minimize,
  Minus,
  Plus,
  RefreshCw,
  Rotate3d,
  RotateCcw,
  RotateCw,
  SlidersHorizontal,
  X,
} from "lucide-react";
import { api, ApiError } from "@/lib/api";
import { humanize } from "@/lib/format";
import { useHydrated } from "@/lib/client-hooks";
import type {
  DesignSnapshot,
  Fabric,
  GarmentType,
  MeasurementProfile,
  OptionGroup,
  SavedDesign,
  StudioConfig,
} from "@/lib/types";
import { useSession } from "@/components/providers/session";
import { useToast } from "@/components/providers/toast";
import { Price } from "@/components/ui/price";
import { FabricSwatch } from "@/components/ui/fabric-swatch";
import { MeasurementForm, measurePayload, type MeasureState } from "@/features/measurements/measurement-form";
import type { FitMarker, RenderStats, ViewAngle } from "./viewer";
import {
  qualityLabel,
  qualityOrder,
  qualitySettings,
  readDeviceInfo,
  startingQuality,
  stepQuality,
  ZOOM,
  type Quality,
} from "./quality";
import styles from "./studio.module.css";

// three.js only loads on this page, after the panel is interactive.
const Viewer = dynamic(() => import("./viewer").then((m) => m.Viewer), {
  ssr: false,
  loading: () => <div className={styles.loading}>Loading the 3D model</div>,
});

type Selections = Record<string, string | number>;
type BaseMm = { height: number; chest: number; waist: number; hip: number; shoulder: number };

// Angles go round the figure: its left side faces the camera at "side", its right at "right".
const views: { key: ViewAngle; label: string; name: string }[] = [
  { key: "front", label: "Front", name: "Front view" },
  { key: "45", label: "45°", name: "45 degree view" },
  { key: "side", label: "Left", name: "Left side view" },
  { key: "135", label: "135°", name: "135 degree view" },
  { key: "back", label: "Back", name: "Back view" },
  { key: "right", label: "Right", name: "Right side view" },
];
const TURN = Math.PI / 8;

const fitLabel: Record<string, string> = {
  good: "Good fit",
  close: "Close fit",
  tight: "Too tight",
  loose: "Too loose",
  insufficient_data: "Needs a measurement",
  verify: "Tailor will check",
};

function defaultsFor(groups: OptionGroup[]): Selections {
  const s: Selections = {};
  for (const g of groups) {
    if (g.selection === "number") {
      if (g.defaultNumber !== null) s[g.key] = g.defaultNumber;
    } else {
      const d = g.values.find((v) => v.isDefault) ?? g.values[0];
      if (d) s[g.key] = d.key;
    }
  }
  return s;
}

// Parts are visible unless a selected option hides them; options then show their own parts.
function visibilityFor(cfg: StudioConfig, sel: Selections): Record<string, boolean> {
  const vis: Record<string, boolean> = {};
  for (const p of cfg.asset?.supportedOptions.baseHidden ?? []) vis[p] = false;
  for (const g of cfg.groups)
    for (const v of g.values) {
      if (sel[g.key] === v.key) continue;
      for (const p of v.assetParts.show ?? []) if (vis[p] === undefined) vis[p] = false;
    }
  for (const g of cfg.groups) {
    const v = g.values.find((x) => x.key === sel[g.key]);
    for (const p of v?.assetParts.hide ?? []) vis[p] = false;
  }
  for (const g of cfg.groups) {
    const v = g.values.find((x) => x.key === sel[g.key]);
    for (const p of v?.assetParts.show ?? []) vis[p] = true;
  }
  return vis;
}

export function Studio({ garments }: { garments: GarmentType[] }) {
  const router = useRouter();
  const sp = useSearchParams();
  const hydrated = useHydrated();
  const { user } = useSession();
  const toast = useToast();
  const designable = garments.filter((g) => g.studioEnabled);
  const [garmentKey, setGarmentKey] = useState(() => {
    const g = sp.get("garment");
    return designable.some((x) => x.key === g) ? g! : (designable[0]?.key ?? "suit");
  });
  const [designId, setDesignId] = useState<string | null>(sp.get("design"));
  const [designVersion, setDesignVersion] = useState<number | null>(null);
  const [selections, setSelections] = useState<Selections>({});
  const [fabricKey, setFabricKey] = useState("");
  const [colorKey, setColorKey] = useState("");
  const [fitPreference, setFitPreference] = useState<"slim" | "regular" | "relaxed">("regular");
  const [baseline, setBaseline] = useState("");
  const [measureMode, setMeasureMode] = useState<"none" | "entered" | "saved">("none");
  const [measure, setMeasure] = useState<MeasureState>({ unit: "cm", height: "", values: {} });
  const [profileVersionId, setProfileVersionId] = useState("");
  const [view, setView] = useState<ViewAngle>("front");
  const [turn, setTurn] = useState(0);
  const [name, setName] = useState("");
  const [saving, setSaving] = useState(false);
  const [quality, setQuality] = useState<Quality>("high");
  const [autoQuality, setAutoQuality] = useState(true);
  const [antialias, setAntialias] = useState(true);
  const [webgl, setWebgl] = useState<boolean | null>(null);
  const [viewerFailed, setViewerFailed] = useState(false);
  const [viewerKey, setViewerKey] = useState(0);
  const [reducedMotion, setReducedMotion] = useState(false);
  const [zoom, setZoom] = useState(ZOOM.initial);
  const [resetKey, setResetKey] = useState(0);
  const [autoRotate, setAutoRotate] = useState(false);
  const [showFit, setShowFit] = useState(false);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const [canFullscreen, setCanFullscreen] = useState(false);
  const [textureFailed, setTextureFailed] = useState(false);
  const [stats, setStats] = useState<RenderStats | null>(null);
  const stage = useRef<HTMLElement>(null);
  const loadedDesign = useRef<string | null>(null);
  // Renderer diagnostics are for staff and development only (?diagnostics=1).
  const diagnostics =
    sp.get("diagnostics") === "1" && (process.env.NODE_ENV !== "production" || Boolean(user?.isStaff));
  const diagnosticsParam = sp.get("diagnostics") === "1" ? "&diagnostics=1" : "";

  useEffect(() => {
    // Device capability checks happen once on the client.
    import("./viewer").then((m) => setWebgl(m.webglAvailable()));
    const start = startingQuality(readDeviceInfo());
    // eslint-disable-next-line react-hooks/set-state-in-effect -- read device capabilities once
    setQuality(start);
    setAntialias(start === "ultra" || start === "high");
    setReducedMotion(window.matchMedia("(prefers-reduced-motion: reduce)").matches);
    setCanFullscreen(Boolean(document.fullscreenEnabled));
    const onFs = () => setFullscreen(document.fullscreenElement === stage.current);
    document.addEventListener("fullscreenchange", onFs);
    return () => document.removeEventListener("fullscreenchange", onFs);
  }, []);

  const onQualityStep = useCallback(
    (dir: "up" | "down") => {
      if (autoQuality) setQuality((q) => stepQuality(q, dir));
    },
    [autoQuality],
  );
  const reset = useCallback(() => {
    setView("front");
    setTurn(0);
    setZoom(ZOOM.initial);
    setAutoRotate(false);
    setResetKey((k) => k + 1);
  }, []);
  const toggleFullscreen = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void stage.current?.requestFullscreen();
  }, []);
  const zoomBy = (d: number) => setZoom((z) => Math.max(ZOOM.min, Math.min(ZOOM.max, z + d)));

  const studio = useQuery({
    queryKey: ["studio", garmentKey],
    queryFn: () => api<StudioConfig>(`/garments/${garmentKey}/studio`),
  });
  const fabrics = useQuery({ queryKey: ["fabrics"], queryFn: () => api<Fabric[]>("/fabrics") });
  const profiles = useQuery({
    queryKey: ["me", "profiles"],
    enabled: Boolean(user?.customerId),
    queryFn: () => api<MeasurementProfile[]>("/me/measurement-profiles"),
  });
  const cfg = studio.data;
  const suitable = useMemo(
    () =>
      (fabrics.data ?? []).filter(
        (f) =>
          f.stockStatus !== "discontinued" && (!f.suitableGarments.length || f.suitableGarments.includes(garmentKey)),
      ),
    [fabrics.data, garmentKey],
  );

  // Defaults when a garment's options first load. A restored design marks its garment as applied.
  const appliedFor = useRef<string | null>(null);
  useEffect(() => {
    if (!cfg || appliedFor.current === cfg.garment.key) return;
    appliedFor.current = cfg.garment.key;

    setSelections(defaultsFor(cfg.groups));
  }, [cfg]);

  // Restore a saved design (?design=).
  useEffect(() => {
    if (!designId || loadedDesign.current === designId) return;
    api<SavedDesign>(`/designs/${designId}`)
      .then((d) => {
        const s: DesignSnapshot = d.snapshot;
        loadedDesign.current = d.id;
        setGarmentKey(s.garment.key);
        setDesignVersion(d.version);
        setName(d.name);
        const sel: Selections = {};
        for (const x of s.selections) sel[x.group] = x.value ?? x.number ?? "";
        appliedFor.current = s.garment.key;
        setSelections(sel);
        if (s.fabric) {
          setFabricKey(s.fabric.key);
          setColorKey(s.fabric.colorKey);
        }
        setFitPreference(
          (["slim", "regular", "relaxed"].includes(s.fitPreference)
            ? s.fitPreference
            : "regular") as typeof fitPreference,
        );
        setBaseline(s.baseline?.type === "size" ? (s.baseline.label ?? "") : "");
        if (s.measurements?.versionId) {
          setMeasureMode("saved");
          setProfileVersionId(s.measurements.versionId);
        } else if (s.measurements) {
          const unit = s.measurements.unit === "in" ? "in" : "cm";
          setMeasureMode("entered");
          setMeasure({
            unit,
            height: s.measurements.heightMm
              ? String(
                  unit === "in" ? Math.round((s.measurements.heightMm / 25.4) * 4) / 4 : s.measurements.heightMm / 10,
                )
              : "",
            values: Object.fromEntries(
              Object.entries(s.measurements.valuesMm).map(([k, mm]) => [
                k,
                String(unit === "in" ? Math.round((mm / 25.4) * 4) / 4 : mm / 10),
              ]),
            ),
          });
        }
      })
      .catch(() => {
        toast("We could not open that design. Starting a new one.", "error");
        setDesignId(null);
      });
  }, [designId, toast]);

  const fabric = suitable.find((f) => f.key === fabricKey) ?? suitable[0] ?? null;
  const color = fabric?.colors.find((c) => c.key === colorKey) ?? fabric?.colors[0] ?? null;
  const asset = cfg?.asset ?? null;
  const bodyModel =
    (asset?.supportedOptions.bodyModel as "masculine" | "feminine" | undefined) ??
    (cfg?.garment.bodyModelHint === "feminine" ? "feminine" : "masculine");
  const body = cfg?.bodyAssets.find((b) => b.supportedOptions.bodyModel === bodyModel) ?? null;
  const lod = qualitySettings[quality].lod;
  const fileFor = (a: typeof asset) => a?.files.find((f) => f.lod === lod)?.url ?? a?.files[0]?.url ?? null;

  const config = useMemo(() => {
    if (!cfg) return null;
    return {
      garment: garmentKey,
      selections,
      fabric: fabric?.key ?? "",
      color: color?.key ?? "",
      baseline,
      fitPreference,
      bodyModel,
      measurementVersionId: measureMode === "saved" && profileVersionId ? profileVersionId : undefined,
      measurements: measureMode === "entered" ? measurePayload(measure) : undefined,
    };
  }, [
    cfg,
    garmentKey,
    selections,
    fabric,
    color,
    baseline,
    fitPreference,
    bodyModel,
    measureMode,
    profileVersionId,
    measure,
  ]);

  // Server-side evaluation keeps price and fit authoritative. Debounced while the customer edits.
  const [debounced, setDebounced] = useState(config);
  useEffect(() => {
    const t = setTimeout(() => setDebounced(config), 300);
    return () => clearTimeout(t);
  }, [config]);
  const evaluation = useQuery({
    queryKey: ["evaluate", debounced],
    enabled: Boolean(debounced && Object.keys(debounced.selections).length),
    queryFn: () => api<DesignSnapshot>("/designs/evaluate", { body: debounced }),
    placeholderData: (prev) => prev,
    retry: false,
  });
  const snap = evaluation.data;

  // Body shape: morph the mannequin and garment from the customer's measurements (or the chosen size).
  const morphs = useMemo(() => {
    const out: Record<string, number> = {};
    const base = asset?.bodyCompat.baseMm as BaseMm | undefined;
    const step = (asset?.bodyCompat.morphStepM as Record<string, number> | undefined) ?? {
      chest: 0.1,
      waist: 0.1,
      hip: 0.1,
      shoulders: 0.05,
    };
    let mm: Record<string, number> = {};
    if (snap?.measurements?.valuesMm) mm = snap.measurements.valuesMm;
    else if (baseline && cfg) {
      const size = cfg.sizes.find((s) => s.label === baseline);
      if (size)
        mm = {
          chest: (size.dims.chest ?? 0) - 100,
          waist: (size.dims.waist ?? 0) - 90,
          hip: (size.dims.hip ?? 0) - 70,
          shoulder: size.dims.shoulders ?? 0,
        };
    }
    if (base) {
      const chest = bodyModel === "feminine" ? (mm.bust ?? mm.chest) : mm.chest;
      const pairs: [string, number | undefined, number, number][] = [
        ["chest", chest, base.chest, step.chest ?? 0.1],
        ["waist", mm.waist, base.waist, step.waist ?? 0.1],
        ["hip", mm.hip, base.hip, step.hip ?? 0.1],
        ["shoulders", mm.shoulder, base.shoulder, step.shoulders ?? 0.05],
      ];
      for (const [k, v, b, st] of pairs) if (v && v > 0) out[k] = Math.max(-1.5, Math.min(2.5, (v - b) / (st * 1000)));
    }
    for (const g of cfg?.groups ?? []) {
      const v = g.values.find((x) => x.key === selections[g.key]);
      for (const [k, w] of Object.entries(v?.assetParts.morphs ?? {})) out[k] = w;
      if (g.selection === "number" && typeof selections[g.key] === "number" && g.defaultNumber) {
        out[g.key] = ((selections[g.key] as number) - g.defaultNumber) / (g.defaultNumber * 0.35);
      }
    }
    return out;
  }, [asset, snap, baseline, cfg, bodyModel, selections]);

  const heightScale = useMemo(() => {
    const base = (asset?.bodyCompat.baseMm as BaseMm | undefined)?.height;
    const h = snap?.measurements?.heightMm;
    if (!base || !h) return 1;
    return Math.max(0.85, Math.min(1.15, h / base));
  }, [asset, snap]);

  const visibility = useMemo(() => (cfg ? visibilityFor(cfg, selections) : {}), [cfg, selections]);

  // Fit results pinned to the figure, at the anchor points the body asset declares for each zone.
  const fitMarkers = useMemo<FitMarker[] | null>(() => {
    if (!showFit || !snap) return null;
    const anchors = (body?.bodyCompat.zoneAnchors ?? asset?.bodyCompat.zoneAnchors) as
      Record<string, [number, number, number]> | undefined;
    if (!anchors) return null;
    return snap.fit.zones.flatMap((z) => {
      const anchor = anchors[z.zone];
      if (z.state === "insufficient_data" || !anchor) return [];
      return [{ zone: z.zone, label: `${z.label}: ${fitLabel[z.state] ?? humanize(z.state)}`, state: z.state, anchor }];
    });
  }, [showFit, snap, body, asset]);
  const canPinFit = Boolean(body?.bodyCompat.zoneAnchors ?? asset?.bodyCompat.zoneAnchors);
  const renders = asset?.supportedOptions.renders;
  const fabricLook = useMemo(
    () => (fabric && color ? { pbr: fabric.pbr, colorHex: color.hex } : null),
    [fabric, color],
  );

  const sections = useMemo(() => {
    const m = new Map<string, OptionGroup[]>();
    for (const g of cfg?.groups ?? []) m.set(g.section, [...(m.get(g.section) ?? []), g]);
    return [...m.entries()];
  }, [cfg]);

  const save = useCallback(async () => {
    if (!config) return null;
    setSaving(true);
    try {
      const body = {
        name: name.trim() || `${cfg?.garment.name ?? "My"} design`,
        config,
        expectedVersion: designVersion ?? undefined,
      };
      const r = designId
        ? await api<{ id: string; version: number }>(`/designs/${designId}`, { method: "PUT", body })
        : await api<{ id: string; version: number }>("/designs", { body });
      loadedDesign.current = r.id;
      setDesignId(r.id);
      setDesignVersion(r.version);
      window.history.replaceState(null, "", `/studio?garment=${garmentKey}&design=${r.id}${diagnosticsParam}`);
      return r.id;
    } catch (e) {
      toast(e instanceof ApiError ? e.message : "We could not save your design. Please try again.", "error");
      return null;
    } finally {
      setSaving(false);
    }
  }, [config, name, cfg, designId, designVersion, garmentKey, toast, diagnosticsParam]);

  if (!hydrated) return <div className={styles.shell} />;

  const label = cfg
    ? `${cfg.garment.name} in ${fabric?.name ?? "fabric"}${color ? `, ${color.name}` : ""}, ${views.find((v) => v.key === view)?.label} view`
    : "Fitting studio";

  return (
    <div className={styles.shell}>
      <section
        ref={stage}
        className={styles.stage}
        aria-label="3D preview"
        tabIndex={0}
        aria-describedby="studio-keys"
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return;
          const k = e.key;
          if (k === "ArrowLeft") setTurn((t) => t - TURN);
          else if (k === "ArrowRight") setTurn((t) => t + TURN);
          else if (k === "ArrowUp" || k === "+" || k === "=") zoomBy(-ZOOM.step);
          else if (k === "ArrowDown" || k === "-") zoomBy(ZOOM.step);
          else if (k === "0" || k === "Home") reset();
          else return;
          e.preventDefault();
        }}
      >
        <p id="studio-keys" className="visually-hidden">
          Use the left and right arrow keys to turn the figure, up and down to zoom, and Home to reset the view.
        </p>
        {webgl === false || viewerFailed ? (
          <StaticPreview
            renders={renders}
            view={view}
            garment={cfg?.garment.name ?? "garment"}
            reason={viewerFailed ? "The 3D preview could not load." : "Your browser cannot show the 3D preview."}
            onRetry={
              viewerFailed && webgl
                ? () => {
                    setViewerFailed(false);
                    setViewerKey((k) => k + 1);
                  }
                : undefined
            }
          />
        ) : cfg && body && asset ? (
          <ViewerBoundary key={viewerKey} onError={() => setViewerFailed(true)}>
            <Viewer
              bodyUrl={fileFor(body)}
              garmentUrl={fileFor(asset)}
              visibility={visibility}
              morphs={morphs}
              heightScale={heightScale}
              fabric={fabricLook}
              view={view}
              turn={turn}
              zoom={zoom}
              resetKey={resetKey}
              autoRotate={autoRotate}
              quality={quality}
              antialias={antialias}
              reducedMotion={reducedMotion}
              fitMarkers={fitMarkers}
              label={label}
              onQualityStep={onQualityStep}
              onZoomChange={setZoom}
              onTextureError={() => setTextureFailed(true)}
              onStats={diagnostics ? setStats : undefined}
              onReset={reset}
            />
          </ViewerBoundary>
        ) : cfg && !asset ? (
          <div className={styles.fallback}>
            <p>A 3D model for this garment is not available yet. Choose your options and we will send a quote.</p>
          </div>
        ) : (
          <div className={styles.loading}>Loading the 3D model</div>
        )}
        <div className={styles.controls}>
          <div className={styles.views} role="group" aria-label="View angle">
            {views
              .filter((v) => (webgl !== false && !viewerFailed ? true : Boolean(renders?.[v.key])))
              .map((v) => (
                <button
                  key={v.key}
                  type="button"
                  aria-label={v.name}
                  aria-pressed={view === v.key && turn === 0}
                  onClick={() => {
                    setTurn(0);
                    setView(v.key);
                  }}
                >
                  {v.label}
                </button>
              ))}
          </div>
          {webgl !== false && !viewerFailed ? (
            <div className={styles.tools} role="group" aria-label="Camera">
              <button
                type="button"
                className="icon-btn"
                aria-label="Turn left"
                onClick={() => setTurn((t) => t - TURN)}
              >
                <RotateCcw size={18} aria-hidden />
              </button>
              <button
                type="button"
                className="icon-btn"
                aria-label="Turn right"
                onClick={() => setTurn((t) => t + TURN)}
              >
                <RotateCw size={18} aria-hidden />
              </button>
              <button
                type="button"
                className="icon-btn"
                aria-label="Zoom in"
                disabled={zoom <= ZOOM.min}
                onClick={() => zoomBy(-ZOOM.step)}
              >
                <Plus size={18} aria-hidden />
              </button>
              <button
                type="button"
                className="icon-btn"
                aria-label="Zoom out"
                disabled={zoom >= ZOOM.max}
                onClick={() => zoomBy(ZOOM.step)}
              >
                <Minus size={18} aria-hidden />
              </button>
              <button type="button" className="icon-btn" aria-label="Reset view" onClick={reset}>
                <RefreshCw size={18} aria-hidden />
              </button>
              <button
                type="button"
                className={styles.toggle}
                aria-pressed={autoRotate}
                onClick={() => setAutoRotate((a) => !a)}
              >
                <Rotate3d size={18} aria-hidden />
                <span className={styles.toggleText}>Turn slowly</span>
              </button>
              {canFullscreen ? (
                <button
                  type="button"
                  className="icon-btn"
                  aria-label={fullscreen ? "Leave full screen" : "Full screen"}
                  aria-pressed={fullscreen}
                  onClick={toggleFullscreen}
                >
                  {fullscreen ? <Minimize size={18} aria-hidden /> : <Maximize size={18} aria-hidden />}
                </button>
              ) : null}
            </div>
          ) : null}
        </div>
        {asset && !asset.productionQuality ? (
          <p className={styles.standin}>
            Simplified preview model. Your garment is cut from your own pattern and its drape and details will differ.
          </p>
        ) : null}
        {textureFailed ? (
          <p className={styles.textureNote} role="status">
            The fabric texture did not load, so the preview shows its plain colour.
          </p>
        ) : null}
        {diagnostics && stats ? (
          <dl className={styles.diagnostics} aria-label="Renderer diagnostics">
            <dt>Level</dt>
            <dd>
              {qualityLabel[quality]}
              {autoQuality ? " (auto)" : ""}
            </dd>
            <dt>Frame</dt>
            <dd>
              {stats.frameMs} ms, {stats.fps} fps
            </dd>
            <dt>Draw calls</dt>
            <dd>{stats.calls}</dd>
            <dt>Triangles</dt>
            <dd>{stats.triangles}</dd>
            <dt>GPU objects</dt>
            <dd>
              {stats.geometries} geometries, {stats.textures} textures
            </dd>
          </dl>
        ) : null}
      </section>

      <aside
        className={styles.panel}
        aria-label="Design options"
        data-open={sheetOpen}
        onKeyDown={(e) => {
          if (e.key === "Escape" && sheetOpen) setSheetOpen(false);
        }}
      >
        <header className={styles.panelHead}>
          <label className="visually-hidden" htmlFor="studio-garment">
            Garment
          </label>
          <select
            id="studio-garment"
            className="select"
            value={garmentKey}
            onChange={(e) => {
              loadedDesign.current = null;
              setDesignId(null);
              setDesignVersion(null);
              setGarmentKey(e.target.value);
              router.replace(`/studio?garment=${e.target.value}${diagnosticsParam}`, { scroll: false });
            }}
          >
            {designable.map((g) => (
              <option key={g.key} value={g.key}>
                {g.name}
              </option>
            ))}
          </select>
          <div className={styles.price} aria-live="polite">
            {snap ? (
              <>
                <span className="tiny muted">Estimate</span>
                <Price minor={snap.price.totalMinor} currency={snap.price.currency} className={styles.priceValue} />
              </>
            ) : (
              <span className="tiny muted">Calculating</span>
            )}
          </div>
          <button
            type="button"
            className={styles.sheetToggle}
            aria-expanded={sheetOpen}
            aria-controls="studio-options"
            onClick={() => setSheetOpen((o) => !o)}
          >
            {sheetOpen ? <X size={18} aria-hidden /> : <SlidersHorizontal size={18} aria-hidden />}
            <span className={styles.sheetToggleText}>{sheetOpen ? "Close" : "Options"}</span>
          </button>
        </header>

        {studio.isError ? (
          <p className="notice notice-danger">
            This garment is not available in the studio.{" "}
            <Link className="link" href="/custom-tailor/request">
              Describe it instead
            </Link>
            .
          </p>
        ) : null}

        <div className={styles.sections} id="studio-options">
          <PanelSection title="Fabric">
            <ul className={styles.fabricList} aria-label="Fabrics">
              {suitable.map((f) => (
                <li key={f.key}>
                  <label className={styles.fabric}>
                    <input
                      type="radio"
                      name="studio-fabric"
                      className="visually-hidden"
                      checked={fabric?.key === f.key}
                      onChange={() => {
                        setFabricKey(f.key);
                        setColorKey(f.colors[0]?.key ?? "");
                      }}
                    />
                    <span className={styles.swatch}>
                      <FabricSwatch
                        fabric={f}
                        colorHex={fabric?.key === f.key ? color?.hex : f.colors[0]?.hex}
                        sizes="64px"
                        round={false}
                      />
                    </span>
                    <span className="small">{f.name}</span>
                  </label>
                </li>
              ))}
            </ul>
            {fabric ? (
              <p className="tiny muted" style={{ margin: 0 }}>
                {fabric.composition}
                {fabric.priceImpactMinor ? (
                  <>
                    {" "}
                    · + <Price minor={fabric.priceImpactMinor} />
                  </>
                ) : null}
              </p>
            ) : null}
            {fabric && fabric.colors.length > 1 ? (
              <fieldset className={styles.fieldset}>
                <legend className="label">Colour</legend>
                <div className="row-wrap">
                  {fabric.colors.map((c) => (
                    <label key={c.key} className={styles.color} title={c.name}>
                      <input
                        type="radio"
                        name="studio-color"
                        className="visually-hidden"
                        checked={color?.key === c.key}
                        onChange={() => setColorKey(c.key)}
                      />
                      <span style={{ background: c.hex }} aria-hidden />
                      <span className="visually-hidden">{c.name}</span>
                    </label>
                  ))}
                  <span className="small muted">{color?.name}</span>
                </div>
              </fieldset>
            ) : null}
          </PanelSection>

          {sections.map(([section, groups]) => (
            <PanelSection key={section} title={humanize(section)}>
              {groups.map((g) =>
                g.selection === "number" ? (
                  <div key={g.key} className={styles.number}>
                    <label htmlFor={`g-${g.key}`} className="label">
                      {g.name}: {String(selections[g.key] ?? g.defaultNumber ?? "")} {g.unit}
                    </label>
                    <input
                      id={`g-${g.key}`}
                      type="range"
                      min={g.minValue ?? 0}
                      max={g.maxValue ?? 10}
                      step={g.stepValue ?? 1}
                      value={Number(selections[g.key] ?? g.defaultNumber ?? 0)}
                      onChange={(e) => setSelections((s) => ({ ...s, [g.key]: Number(e.target.value) }))}
                    />
                  </div>
                ) : (
                  <fieldset key={g.key} className={styles.fieldset}>
                    <legend className="label">{g.name}</legend>
                    <div className={styles.values}>
                      {g.values.map((v) => (
                        <label key={v.key} className={styles.value}>
                          <input
                            type="radio"
                            name={`g-${g.key}`}
                            checked={selections[g.key] === v.key}
                            onChange={() => setSelections((s) => ({ ...s, [g.key]: v.key }))}
                          />
                          <span>{v.name}</span>
                          {v.priceMinor ? (
                            <span className="tiny muted">
                              + <Price minor={v.priceMinor} />
                            </span>
                          ) : null}
                        </label>
                      ))}
                    </div>
                    {(() => {
                      const d = g.values.find((v) => v.key === selections[g.key])?.description;
                      return d ? (
                        <p className="tiny muted" style={{ margin: 0 }}>
                          {d}
                        </p>
                      ) : null;
                    })()}
                  </fieldset>
                ),
              )}
            </PanelSection>
          ))}

          <PanelSection title="Body and fit">
            <fieldset className={styles.fieldset}>
              <legend className="label">Fit</legend>
              <div className={styles.values}>
                {(["slim", "regular", "relaxed"] as const).map((f) => (
                  <label key={f} className={styles.value}>
                    <input type="radio" name="fit" checked={fitPreference === f} onChange={() => setFitPreference(f)} />
                    <span>{humanize(f)}</span>
                  </label>
                ))}
              </div>
            </fieldset>
            <fieldset className={styles.fieldset}>
              <legend className="label">Measurements</legend>
              <div className={styles.values}>
                <label className={styles.value}>
                  <input
                    type="radio"
                    name="mm"
                    checked={measureMode === "none"}
                    onChange={() => setMeasureMode("none")}
                  />
                  <span>Later, or at the studio</span>
                </label>
                <label className={styles.value}>
                  <input
                    type="radio"
                    name="mm"
                    checked={measureMode === "entered"}
                    onChange={() => setMeasureMode("entered")}
                  />
                  <span>Enter mine</span>
                </label>
                {user?.customerId && (profiles.data ?? []).some((p) => p.current) ? (
                  <label className={styles.value}>
                    <input
                      type="radio"
                      name="mm"
                      checked={measureMode === "saved"}
                      onChange={() => setMeasureMode("saved")}
                    />
                    <span>Use saved</span>
                  </label>
                ) : null}
              </div>
            </fieldset>
            {measureMode === "none" && cfg?.sizes.length ? (
              <label className="field">
                <span className="label">Start from a standard size (optional)</span>
                <select className="select" value={baseline} onChange={(e) => setBaseline(e.target.value)}>
                  <option value="">None</option>
                  {cfg.sizes.map((s) => (
                    <option key={s.id} value={s.label}>
                      {s.label}
                    </option>
                  ))}
                </select>
              </label>
            ) : null}
            {measureMode === "saved" ? (
              <select
                className="select"
                aria-label="Saved measurements"
                value={profileVersionId}
                onChange={(e) => setProfileVersionId(e.target.value)}
              >
                <option value="">Choose a profile</option>
                {(profiles.data ?? [])
                  .filter((p) => p.current)
                  .map((p) => (
                    <option key={p.id} value={p.current!.id}>
                      {p.name}
                    </option>
                  ))}
              </select>
            ) : null}
            {measureMode === "entered" && cfg ? (
              <MeasurementForm fields={cfg.measurements} state={measure} onChange={setMeasure} />
            ) : null}
          </PanelSection>

          <PanelSection title="Preview">
            <label className="field">
              <span className="label">Detail</span>
              <select
                className="select"
                value={autoQuality ? "auto" : quality}
                onChange={(e) => {
                  const v = e.target.value;
                  if (v === "auto") {
                    setAutoQuality(true);
                    setQuality(startingQuality(readDeviceInfo()));
                  } else {
                    setAutoQuality(false);
                    setQuality(v as Quality);
                  }
                }}
              >
                <option value="auto">Automatic ({qualityLabel[quality].toLowerCase()})</option>
                {[...qualityOrder].reverse().map((q) => (
                  <option key={q} value={q}>
                    {qualityLabel[q]}
                  </option>
                ))}
              </select>
            </label>
            {canPinFit ? (
              <label className={styles.value}>
                <input type="checkbox" checked={showFit} onChange={(e) => setShowFit(e.target.checked)} />
                <span>Show the fit on the figure</span>
              </label>
            ) : null}
          </PanelSection>

          <PanelSection title="Fit estimate">
            {snap ? (
              <>
                <p className="small" style={{ margin: 0 }}>
                  {snap.fit.summary}
                </p>
                {snap.fit.zones.some((z) => z.state !== "insufficient_data") ? (
                  <ul className={styles.zones}>
                    {snap.fit.zones.map((z) => (
                      <li key={z.zone} data-state={z.state}>
                        <span>{z.label}</span>
                        <span>{fitLabel[z.state] ?? humanize(z.state)}</span>
                      </li>
                    ))}
                  </ul>
                ) : null}
                <p className="tiny muted" style={{ margin: 0 }}>
                  An estimate from your numbers. Your tailor checks every measurement before cutting.
                </p>
              </>
            ) : (
              <p className="small muted">Add your measurements or a size to see how each area should fit.</p>
            )}
            {measureMode === "entered" && evaluation.error instanceof ApiError ? (
              <p className="small" role="alert" style={{ color: "var(--danger)" }}>
                {Object.values(evaluation.error.fields)[0] ?? evaluation.error.message}
              </p>
            ) : null}
          </PanelSection>
        </div>

        <footer className={styles.panelFoot}>
          <div className={styles.footExtra}>
            {snap?.warnings.length ? (
              <ul className={styles.warnings}>
                {snap.warnings.map((w) => (
                  <li key={w}>{w}</li>
                ))}
              </ul>
            ) : null}
            <label className="visually-hidden" htmlFor="design-name">
              Design name
            </label>
            <input
              id="design-name"
              className="input"
              placeholder={`${cfg?.garment.name ?? "My"} design`}
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={80}
            />
          </div>
          <div className={styles.actions}>
            <button
              type="button"
              className="btn"
              disabled={saving || !config}
              onClick={async () => {
                if (await save())
                  toast(
                    user?.customerId
                      ? "Saved to your account."
                      : "Saved on this device. Sign in to keep it in your account.",
                  );
              }}
            >
              Save
            </button>
            <button
              type="button"
              className="btn btn-primary"
              disabled={saving || !config}
              onClick={async () => {
                const id = await save();
                if (id) router.push(`/custom-tailor/request?design=${id}`);
              }}
            >
              Request a quote
            </button>
          </div>
          <p className={`tiny muted ${styles.footExtra}`} style={{ margin: 0 }}>
            No payment now. Your tailor reviews the design and sends a quote.
          </p>
        </footer>
      </aside>
    </div>
  );
}

function PanelSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <details className={styles.section} open>
      <summary>{title}</summary>
      <div className={styles.sectionBody}>{children}</div>
    </details>
  );
}

// StaticPreview stands in for the 3D view when WebGL is missing or the model fails to load. It shows
// pre-rendered angles of the garment's model when the asset has them, and never an unrelated image.
function StaticPreview({
  renders,
  view,
  garment,
  reason,
  onRetry,
}: {
  renders: Record<string, string> | undefined;
  view: ViewAngle;
  garment: string;
  reason: string;
  onRetry?: () => void;
}) {
  const src = renders?.[view] ?? renders?.front;
  const angle = views.find((v) => v.key === (renders?.[view] ? view : "front"))?.name.toLowerCase();
  return (
    <div className={styles.fallback}>
      {src ? (
        // eslint-disable-next-line @next/next/no-img-element -- renders are plain files from the asset pipeline
        <img className={styles.render} src={src} alt={`${garment} preview, ${angle}`} />
      ) : null}
      <div className={styles.fallbackNote}>
        <p>
          {reason}{" "}
          {src
            ? "These pictures show the standard model. Your options and measurements are still saved with the design."
            : "You can still choose every option and send your design for a quote."}
        </p>
        {onRetry ? (
          <button type="button" className="btn" onClick={onRetry}>
            Try again
          </button>
        ) : null}
      </div>
    </div>
  );
}

class ViewerBoundary extends Component<{ children: ReactNode; onError: () => void }, { failed: boolean }> {
  override state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  override componentDidCatch() {
    this.props.onError();
  }
  override render() {
    return this.state.failed ? null : this.props.children;
  }
}
