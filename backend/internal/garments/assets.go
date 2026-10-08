package garments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

func fmtSscan(s string, v *int) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, errors.New("invalid")
	}
	*v = n
	return 1, nil
}

func itoa(i int) string { return strconv.Itoa(i) }

func actor(ctx context.Context) *uuid.UUID { return auth.ActorID(ctx) }

// GLBInfo is what validation learns about a binary glTF file.
type GLBInfo struct {
	Nodes        []string `json:"nodes"`
	Meshes       int      `json:"meshes"`
	MorphTargets []string `json:"morphTargets"`
	Extensions   []string `json:"extensions"`
	Materials    int      `json:"materials"`
}

// InspectGLB validates the GLB container (magic, version 2, chunk layout) and reads its JSON chunk.
func InspectGLB(b []byte) (*GLBInfo, error) {
	if len(b) < 20 || string(b[0:4]) != "glTF" {
		return nil, errors.New("not a binary glTF file")
	}
	if v := binary.LittleEndian.Uint32(b[4:8]); v != 2 {
		return nil, fmt.Errorf("unsupported glTF version %d", v)
	}
	if total := binary.LittleEndian.Uint32(b[8:12]); int(total) != len(b) {
		return nil, errors.New("file length does not match the GLB header")
	}
	jsonLen := binary.LittleEndian.Uint32(b[12:16])
	if string(b[16:20]) != "JSON" || int(jsonLen) > len(b)-20 {
		return nil, errors.New("missing JSON chunk")
	}
	var doc struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
		Meshes []struct {
			Extras struct {
				TargetNames []string `json:"targetNames"`
			} `json:"extras"`
		} `json:"meshes"`
		Materials      []json.RawMessage `json:"materials"`
		ExtensionsUsed []string          `json:"extensionsUsed"`
	}
	if err := json.Unmarshal(b[20:20+jsonLen], &doc); err != nil {
		return nil, errors.New("glTF JSON is invalid")
	}
	info := &GLBInfo{Meshes: len(doc.Meshes), Materials: len(doc.Materials), Extensions: doc.ExtensionsUsed}
	seen := map[string]bool{}
	for _, n := range doc.Nodes {
		info.Nodes = append(info.Nodes, n.Name)
	}
	for _, m := range doc.Meshes {
		for _, t := range m.Extras.TargetNames {
			if !seen[t] {
				seen[t] = true
				info.MorphTargets = append(info.MorphTargets, t)
			}
		}
	}
	return info, nil
}

var allowedExtensions = map[string]bool{
	"KHR_draco_mesh_compression": true, "EXT_meshopt_compression": true, "KHR_texture_basisu": true,
	"KHR_materials_sheen": true, "KHR_materials_clearcoat": true, "KHR_materials_transmission": true,
	"KHR_texture_transform": true, "KHR_mesh_quantization": true, "KHR_materials_specular": true,
	"KHR_materials_ior": true, "KHR_materials_volume": true, "KHR_materials_emissive_strength": true,
}

type AssetsHandler struct {
	Handler
	Store uploads.Storage
}

func (h AssetsHandler) List(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(), `SELECT `+assetCols+` FROM asset_manifests a LEFT JOIN garment_types g ON g.id=a.garment_type_id
		ORDER BY a.asset_key, a.version DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return err
		}
		out = append(out, a)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

// UploadFile accepts a GLB or KTX2 file, validates it and stores it, returning a file descriptor
// with size and checksum for inclusion in a manifest. Only staff with assets.write reach this.
func (h AssetsHandler) UploadFile(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, uploads.MaxAssetBytes+1<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "3D files must be 80 MB or smaller.")
	}
	defer r.MultipartForm.RemoveAll() //nolint:errcheck
	file, _, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest("Choose a file to upload.")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, uploads.MaxAssetBytes+1))
	if err != nil || len(data) > uploads.MaxAssetBytes {
		return httpx.NewError(http.StatusRequestEntityTooLarge, "file_too_large", "3D files must be 80 MB or smaller.")
	}
	var ext, mime string
	var info *GLBInfo
	switch {
	case bytes.HasPrefix(data, []byte("glTF")):
		info, err = InspectGLB(data)
		if err != nil {
			return httpx.Validation(map[string]string{"file": "Invalid GLB: " + err.Error() + "."})
		}
		for _, e := range info.Extensions {
			if !allowedExtensions[e] {
				return httpx.Validation(map[string]string{"file": "Unsupported glTF extension " + e + "."})
			}
		}
		ext, mime = ".glb", "model/gltf-binary"
	case bytes.HasPrefix(data, []byte{0xAB, 'K', 'T', 'X', ' ', '2', '0', 0xBB, '\r', '\n', 0x1A, '\n'}):
		ext, mime = ".ktx2", "image/ktx2"
	default:
		return httpx.Validation(map[string]string{"file": "Upload a GLB model or a KTX2 texture."})
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	key := "assets/" + digest[:2] + "/" + digest + ext
	if err := h.Store.Put(r.Context(), key, bytes.NewReader(data), int64(len(data)), mime); err != nil {
		return err
	}
	_ = audit.Write(r.Context(), h.Pool, audit.Entry{Action: "assets.file.upload", ObjectType: "asset_file", ObjectID: digest,
		After: map[string]any{"bytes": len(data), "info": info}})
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"file": AssetFile{URL: "/api/v1/asset-files/" + digest + ext, Bytes: int64(len(data)), SHA256: digest},
		"info": info,
	})
	return nil
}

// ServeFile serves content-addressed asset files with immutable caching.
func (h AssetsHandler) ServeFile(w http.ResponseWriter, r *http.Request) error {
	name := chi.URLParam(r, "name")
	digest, ext, ok := strings.Cut(name, ".")
	if !ok || len(digest) != 64 || (ext != "glb" && ext != "ktx2") {
		return httpx.NotFound("Asset not found.")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return httpx.NotFound("Asset not found.")
	}
	obj, err := h.Store.Get(r.Context(), "assets/"+digest[:2]+"/"+name)
	if errors.Is(err, uploads.ErrObjectNotFound) {
		return httpx.NotFound("Asset not found.")
	}
	if err != nil {
		return err
	}
	defer obj.Close()
	mime := "model/gltf-binary"
	if ext == "ktx2" {
		mime = "image/ktx2"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	_, err = io.Copy(w, obj)
	return err
}

type manifestInput struct {
	AssetKey          string          `json:"assetKey"`
	Kind              string          `json:"kind"`
	GarmentTypeKey    *string         `json:"garmentTypeKey"`
	Files             []AssetFile     `json:"files"`
	BodyCompat        json.RawMessage `json:"bodyCompat"`
	SupportedOptions  json.RawMessage `json:"supportedOptions"`
	TextureSetVersion string          `json:"textureSetVersion"`
	License           json.RawMessage `json:"license"`
	ProductionQuality bool            `json:"productionQuality"`
	Notes             string          `json:"notes"`
}

// CreateManifest registers a new asset version in the "uploaded" state. It never goes live automatically.
func (h AssetsHandler) CreateManifest(w http.ResponseWriter, r *http.Request) error {
	var in manifestInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	if !keyRe.MatchString(in.AssetKey) {
		f.Add("assetKey", "Use lowercase letters, numbers or hyphens.")
	}
	switch in.Kind {
	case "body", "garment", "texture_pack", "preview_render":
	default:
		f.Add("kind", "Choose an asset kind.")
	}
	if len(in.Files) == 0 {
		f.Add("files", "Add at least one file.")
	}
	for i, fl := range in.Files {
		if fl.LOD == "" || len(fl.SHA256) != 64 || fl.Bytes <= 0 || !strings.HasPrefix(fl.URL, "/") {
			f.Add("files."+itoa(i), "Each file needs a level of detail, URL, size and checksum.")
		}
	}
	var lic struct {
		Source  string `json:"source"`
		License string `json:"license"`
	}
	if err := json.Unmarshal(orEmpty(in.License), &lic); err != nil || lic.Source == "" || lic.License == "" {
		f.Add("license", "Record the asset source and license.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var garmentID *uuid.UUID
		if in.GarmentTypeKey != nil && *in.GarmentTypeKey != "" {
			g, err := GetType(ctx, tx, *in.GarmentTypeKey)
			if err != nil {
				return httpx.Validation(map[string]string{"garmentTypeKey": "Unknown garment type."})
			}
			garmentID = &g.ID
		}
		var version int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(version),0)+1 FROM asset_manifests WHERE asset_key=$1`, in.AssetKey).Scan(&version); err != nil {
			return err
		}
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO asset_manifests (asset_key, version, kind, garment_type_id, status, files, body_compat,
				supported_options, texture_set_version, license, production_quality, notes, created_by)
			VALUES ($1,$2,$3,$4,'uploaded',$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`,
			in.AssetKey, version, in.Kind, garmentID, in.Files, []byte(orEmpty(in.BodyCompat)), []byte(orEmpty(in.SupportedOptions)),
			in.TextureSetVersion, []byte(orEmpty(in.License)), in.ProductionQuality, in.Notes, actor(ctx)).Scan(&id); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "assets.manifest.create", ObjectType: "asset_manifest", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusCreated, map[string]any{"id": id, "version": version})
		return nil
	})
}

func orEmpty(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(`{}`)
	}
	return b
}

// Asset pipeline: uploaded -> validating -> processed -> preview -> approved -> published -> archived.
// Any non-terminal state can be rejected. Publishing archives the previously published version.
var assetTransitions = map[string][]string{
	"uploaded":   {"validating", "rejected"},
	"validating": {"processed", "rejected"},
	"processed":  {"preview", "rejected"},
	"preview":    {"approved", "rejected"},
	"approved":   {"published", "rejected"},
	"published":  {"archived"},
	"archived":   {"published"},
	"rejected":   {},
}

func CanTransitionAsset(from, to string) bool {
	for _, s := range assetTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

func (h AssetsHandler) Transition(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var cur, key string
		if err := tx.QueryRow(ctx, `SELECT status, asset_key FROM asset_manifests WHERE id=$1 FOR UPDATE`, id).Scan(&cur, &key); err != nil {
			return err
		}
		if !CanTransitionAsset(cur, in.Status) {
			return httpx.Conflict("invalid_transition", fmt.Sprintf("An asset cannot move from %s to %s.", cur, in.Status))
		}
		if in.Status == "validating" || in.Status == "processed" {
			if err := h.verifyFiles(ctx, tx, id); err != nil {
				return err
			}
		}
		if in.Status == "published" {
			if _, err := tx.Exec(ctx, `UPDATE asset_manifests SET status='archived' WHERE asset_key=$1 AND status='published' AND id<>$2`, key, id); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE asset_manifests SET status=$2, notes = CASE WHEN $3 = '' THEN notes ELSE notes || E'\n' || $3 END WHERE id=$1`,
			id, in.Status, strings.TrimSpace(in.Note)); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "assets.manifest.transition", ObjectType: "asset_manifest", ObjectID: id.String(),
			Before: map[string]string{"status": cur}, After: map[string]string{"status": in.Status, "note": in.Note}}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

// verifyFiles re-reads stored files and checks checksums and sizes against the manifest.
// Files bundled with the frontend (URLs outside /api/) are verified at build time instead.
func (h AssetsHandler) verifyFiles(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var files []AssetFile
	if err := tx.QueryRow(ctx, `SELECT files FROM asset_manifests WHERE id=$1`, id).Scan(&files); err != nil {
		return err
	}
	for _, f := range files {
		name, ok := strings.CutPrefix(f.URL, "/api/v1/asset-files/")
		if !ok {
			continue
		}
		obj, err := h.Store.Get(ctx, "assets/"+name[:2]+"/"+name)
		if err != nil {
			return httpx.Conflict("asset_missing", "A file in this manifest is missing from storage.")
		}
		hsh := sha256.New()
		n, err := io.Copy(hsh, obj)
		obj.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(hsh.Sum(nil)) != f.SHA256 || n != f.Bytes {
			return httpx.Conflict("checksum_mismatch", "A file does not match its recorded checksum.")
		}
	}
	return nil
}
