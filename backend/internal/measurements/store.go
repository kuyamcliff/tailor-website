package measurements

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

// Fields returns the active measurement fields for a garment type (in its order), or all active fields.
func Fields(ctx context.Context, q db.Querier, garmentKey string) ([]Field, error) {
	var rows pgx.Rows
	var err error
	if garmentKey == "" {
		rows, err = q.Query(ctx, `SELECT key, label, body_location, instruction, helper_note, diagram_key, kind, min_mm, max_mm, false, sort_order
			FROM measurement_fields WHERE active ORDER BY sort_order, key`)
	} else {
		rows, err = q.Query(ctx, `SELECT f.key, f.label, f.body_location, f.instruction, f.helper_note, f.diagram_key, f.kind, f.min_mm, f.max_mm,
				gmf.required, gmf.sort_order
			FROM garment_measurement_fields gmf
			JOIN garment_types g ON g.id = gmf.garment_type_id
			JOIN measurement_fields f ON f.id = gmf.field_id
			WHERE g.key = $1 AND f.active ORDER BY gmf.sort_order, f.key`, garmentKey)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Field{}
	for rows.Next() {
		var f Field
		if err := rows.Scan(&f.Key, &f.Label, &f.BodyLocation, &f.Instruction, &f.HelperNote, &f.DiagramKey, &f.Kind, &f.MinMM, &f.MaxMM, &f.Required, &f.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type Version struct {
	ID            uuid.UUID          `json:"id"`
	ProfileID     *uuid.UUID         `json:"profileId"`
	VersionNo     int                `json:"versionNo"`
	Source        string             `json:"source"`
	HeightMM      *int               `json:"heightMm"`
	BodyModel     string             `json:"bodyModel"`
	FitPreference string             `json:"fitPreference"`
	ValuesMM      map[string]int     `json:"valuesMm"`
	Entered       map[string]float64 `json:"entered"`
	Unit          string             `json:"unit"`
	Notes         string             `json:"notes"`
	ReviewFlags   []Flag             `json:"reviewFlags"`
	VerifiedBy    *string            `json:"verifiedBy"`
	VerifiedAt    *time.Time         `json:"verifiedAt"`
	CreatedAt     time.Time          `json:"createdAt"`
}

const versionCols = `v.id, v.profile_id, v.version_no, v.source, v.height_mm, v.body_model, v.fit_preference, v.values_mm, v.entered,
	v.unit, v.notes, v.review_flags, vu.full_name, v.verified_at, v.created_at`

func scanVersion(row pgx.Row) (*Version, error) {
	v := &Version{}
	err := row.Scan(&v.ID, &v.ProfileID, &v.VersionNo, &v.Source, &v.HeightMM, &v.BodyModel, &v.FitPreference, &v.ValuesMM, &v.Entered,
		&v.Unit, &v.Notes, &v.ReviewFlags, &v.VerifiedBy, &v.VerifiedAt, &v.CreatedAt)
	if v.ReviewFlags == nil {
		v.ReviewFlags = []Flag{}
	}
	return v, err
}

func GetVersion(ctx context.Context, q db.Querier, id uuid.UUID) (*Version, error) {
	return scanVersion(q.QueryRow(ctx, `SELECT `+versionCols+` FROM measurement_versions v
		LEFT JOIN users vu ON vu.id = v.verified_by WHERE v.id=$1`, id))
}

type NewVersion struct {
	ProfileID     *uuid.UUID
	CustomerID    *uuid.UUID
	Source        string
	BodyModel     string
	FitPreference string
	Unit          string
	Validated     Validated
	Notes         string
	Verified      bool
}

// CreateVersion inserts an immutable measurement snapshot and, for profile versions, makes it current.
func CreateVersion(ctx context.Context, tx pgx.Tx, nv NewVersion) (uuid.UUID, error) {
	versionNo := 1
	if nv.ProfileID != nil {
		// Lock the profile row so concurrent saves from two devices get distinct version numbers.
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT max(version_no) FROM measurement_versions WHERE profile_id=$1),0)+1
			FROM measurement_profiles WHERE id=$1 FOR UPDATE`, *nv.ProfileID).Scan(&versionNo); err != nil {
			return uuid.Nil, err
		}
	}
	var verifiedBy *uuid.UUID
	var verifiedAt *time.Time
	if nv.Verified {
		verifiedBy = auth.ActorID(ctx)
		now := time.Now()
		verifiedAt = &now
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO measurement_versions (profile_id, customer_id, version_no, source, height_mm, body_model, fit_preference,
		values_mm, entered, unit, notes, review_flags, created_by, verified_by, verified_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id`,
		nv.ProfileID, nv.CustomerID, versionNo, nv.Source, nv.Validated.HeightMM, nv.BodyModel, nv.FitPreference,
		nv.Validated.ValuesMM, nv.Validated.Entered, nv.Unit, nv.Notes, nv.Validated.Flags, auth.ActorID(ctx), verifiedBy, verifiedAt).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	if nv.ProfileID != nil {
		if _, err := tx.Exec(ctx, `UPDATE measurement_profiles SET current_version_id=$2 WHERE id=$1`, *nv.ProfileID, id); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

type Profile struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	IsDefault     bool      `json:"isDefault"`
	BodyModel     string    `json:"bodyModel"`
	Unit          string    `json:"unit"`
	AgeRange      *string   `json:"ageRange"`
	FitPreference string    `json:"fitPreference"`
	Current       *Version  `json:"current"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

func ListProfiles(ctx context.Context, q db.Querier, customerID uuid.UUID) ([]Profile, error) {
	rows, err := q.Query(ctx, `SELECT id, name, is_default, body_model, unit, age_range, fit_preference, current_version_id, updated_at
		FROM measurement_profiles WHERE customer_id=$1 AND deleted_at IS NULL ORDER BY is_default DESC, created_at`, customerID)
	if err != nil {
		return nil, err
	}
	type row struct {
		p   Profile
		cur *uuid.UUID
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.p.ID, &r.p.Name, &r.p.IsDefault, &r.p.BodyModel, &r.p.Unit, &r.p.AgeRange, &r.p.FitPreference, &r.cur, &r.p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, r)
	}
	rows.Close()
	out := make([]Profile, 0, len(list))
	for _, r := range list {
		if r.cur != nil {
			v, err := GetVersion(ctx, q, *r.cur)
			if err != nil {
				return nil, err
			}
			r.p.Current = v
		}
		out = append(out, r.p)
	}
	return out, nil
}

func Versions(ctx context.Context, q db.Querier, profileID uuid.UUID) ([]Version, error) {
	rows, err := q.Query(ctx, `SELECT `+versionCols+` FROM measurement_versions v LEFT JOIN users vu ON vu.id = v.verified_by
		WHERE v.profile_id=$1 ORDER BY v.version_no DESC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Version{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

type Handler struct{ Pool *pgxpool.Pool }

// ListFields serves the guided measurement definitions for a garment type.
func (h Handler) ListFields(w http.ResponseWriter, r *http.Request) error {
	fields, err := Fields(r.Context(), h.Pool, r.URL.Query().Get("garment"))
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
	httpx.JSON(w, http.StatusOK, fields)
	return nil
}

type validateRequest struct {
	Garment string             `json:"garment"`
	Unit    string             `json:"unit"`
	Height  *float64           `json:"height"`
	Values  map[string]float64 `json:"values"`
	Require bool               `json:"requireAll"`
}

// ValidateEndpoint lets the studio validate and normalize measurements before saving, including for guests.
func (h Handler) ValidateEndpoint(w http.ResponseWriter, r *http.Request) error {
	var req validateRequest
	if err := httpx.Decode(r, &req); err != nil {
		return err
	}
	fields, err := Fields(r.Context(), h.Pool, req.Garment)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, Validate(fields, req.Values, req.Unit, req.Height, req.Require))
	return nil
}

func customerID(r *http.Request) (uuid.UUID, error) {
	p := auth.FromContext(r.Context())
	if p == nil || p.CustomerID == nil {
		return uuid.Nil, httpx.Unauthorized()
	}
	return *p.CustomerID, nil
}

func (h Handler) MyProfiles(w http.ResponseWriter, r *http.Request) error {
	cid, err := customerID(r)
	if err != nil {
		return err
	}
	list, err := ListProfiles(r.Context(), h.Pool, cid)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

type profileInput struct {
	Name          string  `json:"name"`
	BodyModel     string  `json:"bodyModel"`
	Unit          string  `json:"unit"`
	AgeRange      *string `json:"ageRange"`
	FitPreference string  `json:"fitPreference"`
	IsDefault     bool    `json:"isDefault"`
}

func (in *profileInput) validate() error {
	f := httpx.Fields{}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		f.Add("name", "Give this profile a short name.")
	}
	if in.BodyModel != "masculine" && in.BodyModel != "feminine" {
		f.Add("bodyModel", "Choose a body model.")
	}
	if in.Unit != "cm" && in.Unit != "in" {
		f.Add("unit", "Choose centimetres or inches.")
	}
	if in.FitPreference != "slim" && in.FitPreference != "regular" && in.FitPreference != "relaxed" {
		f.Add("fitPreference", "Choose a fit preference.")
	}
	if in.AgeRange != nil {
		allowed := map[string]bool{"under_18": true, "18_29": true, "30_44": true, "45_59": true, "60_plus": true}
		if *in.AgeRange == "" {
			in.AgeRange = nil
		} else if !allowed[*in.AgeRange] {
			f.Add("ageRange", "Choose an age range or leave it empty.")
		}
	}
	return f.Err()
}

func (h Handler) CreateProfile(w http.ResponseWriter, r *http.Request) error {
	cid, err := customerID(r)
	if err != nil {
		return err
	}
	var in profileInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	ctx := r.Context()
	var id uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM measurement_profiles WHERE customer_id=$1 AND deleted_at IS NULL`, cid).Scan(&count); err != nil {
			return err
		}
		if count >= 10 {
			return httpx.Conflict("limit_reached", "You can keep up to 10 measurement profiles.")
		}
		isDefault := in.IsDefault || count == 0
		if isDefault {
			if _, err := tx.Exec(ctx, `UPDATE measurement_profiles SET is_default=false WHERE customer_id=$1 AND is_default`, cid); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, `INSERT INTO measurement_profiles (customer_id, name, is_default, body_model, unit, age_range, fit_preference)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, cid, in.Name, isDefault, in.BodyModel, in.Unit, in.AgeRange, in.FitPreference).Scan(&id)
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
	return nil
}

func (h Handler) ownedProfile(ctx context.Context, r *http.Request) (uuid.UUID, uuid.UUID, error) {
	cid, err := customerID(r)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	var ok bool
	if err := h.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM measurement_profiles WHERE id=$1 AND customer_id=$2 AND deleted_at IS NULL)`, id, cid).Scan(&ok); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if !ok {
		return uuid.Nil, uuid.Nil, httpx.NotFound("Measurement profile not found.")
	}
	return cid, id, nil
}

func (h Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	cid, id, err := h.ownedProfile(ctx, r)
	if err != nil {
		return err
	}
	var in profileInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if in.IsDefault {
			if _, err := tx.Exec(ctx, `UPDATE measurement_profiles SET is_default=false WHERE customer_id=$1 AND is_default AND id<>$2`, cid, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE measurement_profiles SET name=$2, body_model=$3, unit=$4, age_range=$5, fit_preference=$6,
			is_default = (is_default OR $7) WHERE id=$1`, id, in.Name, in.BodyModel, in.Unit, in.AgeRange, in.FitPreference, in.IsDefault)
		return err
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// DeleteProfile soft-deletes the profile. Versions referenced by orders remain for order history.
func (h Handler) DeleteProfile(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	_, id, err := h.ownedProfile(ctx, r)
	if err != nil {
		return err
	}
	if _, err := h.Pool.Exec(ctx, `UPDATE measurement_profiles SET deleted_at=now(), is_default=false WHERE id=$1`, id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type versionInput struct {
	Garment string             `json:"garment"`
	Unit    string             `json:"unit"`
	Height  *float64           `json:"height"`
	Values  map[string]float64 `json:"values"`
	Notes   string             `json:"notes"`
}

// AddVersion saves new customer-entered measurements as a new immutable version.
// Existing orders keep referencing the version they were placed with.
func (h Handler) AddVersion(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	cid, id, err := h.ownedProfile(ctx, r)
	if err != nil {
		return err
	}
	id2, v, err := h.addVersion(ctx, r, id, &cid, "customer_entered", false)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id2, "reviewFlags": v.Flags})
	return nil
}

func (h Handler) addVersion(ctx context.Context, r *http.Request, profileID uuid.UUID, cid *uuid.UUID, source string, verified bool) (uuid.UUID, Validated, error) {
	var in versionInput
	if err := httpx.Decode(r, &in); err != nil {
		return uuid.Nil, Validated{}, err
	}
	if len(in.Notes) > 1000 {
		return uuid.Nil, Validated{}, httpx.Validation(map[string]string{"notes": "Notes are too long."})
	}
	fields, err := Fields(ctx, h.Pool, in.Garment)
	if err != nil {
		return uuid.Nil, Validated{}, err
	}
	if in.Garment != "" && len(fields) == 0 {
		return uuid.Nil, Validated{}, httpx.Validation(map[string]string{"garment": "Unknown garment."})
	}
	// Merge with the current version so measurements for other garments are kept.
	var bodyModel, fit string
	var cur *uuid.UUID
	if err := h.Pool.QueryRow(ctx, `SELECT body_model, fit_preference, current_version_id FROM measurement_profiles WHERE id=$1`, profileID).
		Scan(&bodyModel, &fit, &cur); err != nil {
		return uuid.Nil, Validated{}, err
	}
	v := Validate(fields, in.Values, in.Unit, in.Height, false)
	if len(v.Errors) > 0 {
		return uuid.Nil, v, httpx.Validation(v.Errors)
	}
	if len(v.ValuesMM) == 0 && v.HeightMM == nil {
		return uuid.Nil, v, httpx.Validation(map[string]string{"values": "Enter at least one measurement."})
	}
	if cur != nil {
		prev, err := GetVersion(ctx, h.Pool, *cur)
		if err != nil {
			return uuid.Nil, v, err
		}
		for k, mm := range prev.ValuesMM {
			if _, ok := v.ValuesMM[k]; !ok {
				v.ValuesMM[k] = mm
				if e, ok := prev.Entered[k]; ok && prev.Unit == in.Unit {
					v.Entered[k] = e
				} else {
					v.Entered[k] = FromMM(mm, in.Unit)
				}
			}
		}
		if v.HeightMM == nil {
			v.HeightMM = prev.HeightMM
		}
		v.Flags = ReviewFlags(v.ValuesMM, v.HeightMM)
	}
	var newID uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var err error
		newID, err = CreateVersion(ctx, tx, NewVersion{ProfileID: &profileID, CustomerID: cid, Source: source, BodyModel: bodyModel,
			FitPreference: fit, Unit: in.Unit, Validated: v, Notes: strings.TrimSpace(in.Notes), Verified: verified})
		if err != nil {
			return err
		}
		if verified {
			return audit.Write(ctx, tx, audit.Entry{Action: "measurements.verify", ObjectType: "measurement_profile",
				ObjectID: profileID.String(), After: map[string]any{"versionId": newID, "values": v.ValuesMM, "heightMm": v.HeightMM}})
		}
		return nil
	})
	return newID, v, err
}

func (h Handler) ProfileVersions(w http.ResponseWriter, r *http.Request) error {
	_, id, err := h.ownedProfile(r.Context(), r)
	if err != nil {
		return err
	}
	list, err := Versions(r.Context(), h.Pool, id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

// Staff endpoints.

func (h Handler) CustomerProfiles(w http.ResponseWriter, r *http.Request) error {
	cid, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	list, err := ListProfiles(r.Context(), h.Pool, cid)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

func (h Handler) StaffProfileVersions(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	list, err := Versions(r.Context(), h.Pool, id)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, list)
	return nil
}

// StaffCreateProfile lets a tailor create a profile for a customer (e.g. after an in-store measuring session).
func (h Handler) StaffCreateProfile(w http.ResponseWriter, r *http.Request) error {
	cid, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in profileInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.validate(); err != nil {
		return err
	}
	ctx := r.Context()
	var id uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var hasDefault bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM measurement_profiles WHERE customer_id=$1 AND is_default AND deleted_at IS NULL)`, cid).Scan(&hasDefault); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO measurement_profiles (customer_id, name, is_default, body_model, unit, age_range, fit_preference)
			VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, cid, in.Name, !hasDefault, in.BodyModel, in.Unit, in.AgeRange, in.FitPreference).Scan(&id); err != nil {
			if db.IsForeignKeyViolation(err) {
				return httpx.NotFound("Customer not found.")
			}
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "measurements.profile.create", ObjectType: "measurement_profile", ObjectID: id.String(),
			After: map[string]any{"customerId": cid, "name": in.Name}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
	return nil
}

// StaffAddVersion records tailor-verified measurements as a new version.
func (h Handler) StaffAddVersion(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var cid uuid.UUID
	if err := h.Pool.QueryRow(ctx, `SELECT customer_id FROM measurement_profiles WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&cid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound("Measurement profile not found.")
		}
		return err
	}
	newID, v, err := h.addVersion(ctx, r, id, &cid, "tailor_verified", true)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": newID, "reviewFlags": v.Flags})
	return nil
}

// StaffVersion returns any measurement version (for request and order review).
func (h Handler) StaffVersion(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	v, err := GetVersion(r.Context(), h.Pool, id)
	if err != nil {
		return err
	}
	fields, err := Fields(r.Context(), h.Pool, "")
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"version": v, "fields": fields})
	return nil
}
