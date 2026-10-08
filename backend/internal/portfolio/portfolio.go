// Package portfolio manages the atelier's published work and customer testimonials.
// Testimonials are only ever entered by staff from real customers, with recorded consent.
package portfolio

import (
	"context"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

var Categories = map[string]bool{"suits": true, "dresses": true, "gowns": true, "shirts": true, "trousers": true,
	"traditional": true, "wedding": true, "alterations": true, "other": true}

type Media struct {
	ID       uuid.UUID  `json:"id"`
	URL      string     `json:"url"`
	UploadID *uuid.UUID `json:"uploadId"`
	Alt      string     `json:"alt"`
	Width    *int       `json:"width"`
	Height   *int       `json:"height"`
}

type Project struct {
	ID                 uuid.UUID `json:"id"`
	Slug               string    `json:"slug"`
	Title              string    `json:"title"`
	Category           string    `json:"category"`
	Description        string    `json:"description"`
	Materials          string    `json:"materials"`
	Tags               []string  `json:"tags"`
	CustomerPermission string    `json:"customerPermission,omitempty"`
	VideoURL           *string   `json:"videoUrl"`
	Featured           bool      `json:"featured"`
	Status             string    `json:"status,omitempty"`
	SortOrder          int       `json:"sortOrder"`
	Media              []Media   `json:"media"`
	CreatedAt          time.Time `json:"createdAt"`
}

func list(ctx context.Context, q db.Querier, category string, featured, staff bool, limit int) ([]Project, error) {
	rows, err := q.Query(ctx, `SELECT id, slug, title, category, description, materials, tags, customer_permission, video_url, featured, status, sort_order, created_at
		FROM portfolio_projects WHERE ($1 OR status='published') AND ($2 = '' OR category=$2) AND (NOT $3 OR featured)
		ORDER BY featured DESC, sort_order, created_at DESC LIMIT $4`, staff, category, featured, limit)
	if err != nil {
		return nil, err
	}
	var out []Project
	idx := map[uuid.UUID]int{}
	var ids []uuid.UUID
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.Slug, &p.Title, &p.Category, &p.Description, &p.Materials, &p.Tags, &p.CustomerPermission,
			&p.VideoURL, &p.Featured, &p.Status, &p.SortOrder, &p.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if !staff {
			p.CustomerPermission, p.Status = "", ""
		}
		p.Media = []Media{}
		idx[p.ID] = len(out)
		ids = append(ids, p.ID)
		out = append(out, p)
	}
	rows.Close()
	if len(ids) == 0 {
		return []Project{}, nil
	}
	mrows, err := q.Query(ctx, `SELECT project_id, id, url, upload_id, alt, width, height FROM portfolio_media WHERE project_id = ANY($1) ORDER BY sort_order`, ids)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var pid uuid.UUID
		var m Media
		if err := mrows.Scan(&pid, &m.ID, &m.URL, &m.UploadID, &m.Alt, &m.Width, &m.Height); err != nil {
			return nil, err
		}
		out[idx[pid]].Media = append(out[idx[pid]].Media, m)
	}
	return out, mrows.Err()
}

type Handler struct{ Pool *pgxpool.Pool }

func (h Handler) PublicList(w http.ResponseWriter, r *http.Request) error {
	limit := 60
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v < 60 {
		limit = v
	}
	out, err := list(r.Context(), h.Pool, r.URL.Query().Get("category"), r.URL.Query().Get("featured") == "true", false, limit)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

func (h Handler) PublicGet(w http.ResponseWriter, r *http.Request) error {
	all, err := list(r.Context(), h.Pool, "", false, false, 500)
	if err != nil {
		return err
	}
	slug := chi.URLParam(r, "slug")
	for _, p := range all {
		if p.Slug == slug {
			httpx.JSON(w, http.StatusOK, p)
			return nil
		}
	}
	return httpx.NotFound("This project could not be found.")
}

func (h Handler) OwnerList(w http.ResponseWriter, r *http.Request) error {
	out, err := list(r.Context(), h.Pool, "", false, true, 500)
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, out)
	return nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type projectInput struct {
	Slug               string   `json:"slug"`
	Title              string   `json:"title"`
	Category           string   `json:"category"`
	Description        string   `json:"description"`
	Materials          string   `json:"materials"`
	Tags               []string `json:"tags"`
	CustomerPermission string   `json:"customerPermission"`
	VideoURL           *string  `json:"videoUrl"`
	Featured           bool     `json:"featured"`
	Status             string   `json:"status"`
	SortOrder          int      `json:"sortOrder"`
	Media              []struct {
		UploadID *uuid.UUID `json:"uploadId"`
		URL      string     `json:"url"`
		Alt      string     `json:"alt"`
		Width    *int       `json:"width"`
		Height   *int       `json:"height"`
	} `json:"media"`
}

func (h Handler) OwnerSave(w http.ResponseWriter, r *http.Request) error {
	var in projectInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	if !slugRe.MatchString(in.Slug) {
		f.Add("slug", "Use lowercase words separated by hyphens.")
	}
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		f.Add("title", "Enter a title.")
	}
	if !Categories[in.Category] {
		f.Add("category", "Choose a category.")
	}
	switch in.CustomerPermission {
	case "not_required", "granted", "pending", "denied":
	default:
		f.Add("customerPermission", "Record whether the customer gave permission.")
	}
	if in.Status != "draft" && in.Status != "published" {
		f.Add("status", "Choose draft or published.")
	}
	if in.Status == "published" && in.CustomerPermission != "not_required" && in.CustomerPermission != "granted" {
		f.Add("customerPermission", "A project can only be published once the customer has given permission.")
	}
	if in.Status == "published" && len(in.Media) == 0 {
		f.Add("media", "Add at least one photo before publishing.")
	}
	if in.VideoURL != nil && *in.VideoURL != "" && !strings.HasPrefix(*in.VideoURL, "https://") {
		f.Add("videoUrl", "Video links must use https.")
	}
	for i, m := range in.Media {
		if strings.TrimSpace(m.Alt) == "" {
			f.Add("media."+strconv.Itoa(i), "Describe each image.")
		}
		if m.UploadID == nil && !strings.HasPrefix(m.URL, "/") {
			f.Add("media."+strconv.Itoa(i), "Use an uploaded image.")
		}
	}
	if err := f.Err(); err != nil {
		return err
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	ctx := r.Context()
	idParam := chi.URLParam(r, "id")
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		var err error
		if idParam == "" {
			err = tx.QueryRow(ctx, `INSERT INTO portfolio_projects (slug, title, category, description, materials, tags, customer_permission, video_url, featured, status, sort_order)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, in.Slug, in.Title, in.Category, in.Description, in.Materials, in.Tags,
				in.CustomerPermission, in.VideoURL, in.Featured, in.Status, in.SortOrder).Scan(&id)
		} else {
			if id, err = uuid.Parse(idParam); err != nil {
				return httpx.NotFound("Project not found.")
			}
			var tag interface{ RowsAffected() int64 }
			tag, err = tx.Exec(ctx, `UPDATE portfolio_projects SET slug=$2, title=$3, category=$4, description=$5, materials=$6, tags=$7,
				customer_permission=$8, video_url=$9, featured=$10, status=$11, sort_order=$12 WHERE id=$1`, id, in.Slug, in.Title, in.Category,
				in.Description, in.Materials, in.Tags, in.CustomerPermission, in.VideoURL, in.Featured, in.Status, in.SortOrder)
			if err == nil && tag.RowsAffected() == 0 {
				return httpx.NotFound("Project not found.")
			}
		}
		if db.IsUniqueViolation(err, "") {
			return httpx.Validation(map[string]string{"slug": "Another project already uses this slug."})
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM portfolio_media WHERE project_id=$1`, id); err != nil {
			return err
		}
		for i, m := range in.Media {
			url := m.URL
			if m.UploadID != nil {
				url = "/api/v1/uploads/" + m.UploadID.String() + "/preview"
			}
			if _, err := tx.Exec(ctx, `INSERT INTO portfolio_media (project_id, url, upload_id, alt, width, height, sort_order) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
				id, url, m.UploadID, strings.TrimSpace(m.Alt), m.Width, m.Height, i); err != nil {
				return err
			}
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "portfolio.save", ObjectType: "portfolio_project", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"id": id})
		return nil
	})
}

func (h Handler) OwnerDelete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM portfolio_projects WHERE id=$1`, id); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "portfolio.delete", ObjectType: "portfolio_project", ObjectID: id.String()}); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	})
}

// ---- Testimonials ----

type Testimonial struct {
	ID           uuid.UUID  `json:"id"`
	CustomerName string     `json:"customerName"`
	Quote        string     `json:"quote"`
	Context      string     `json:"context"`
	Source       string     `json:"source,omitempty"`
	Consent      bool       `json:"consent,omitempty"`
	Status       string     `json:"status,omitempty"`
	PublishedAt  *time.Time `json:"publishedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
}

func (h Handler) PublicTestimonials(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(), `SELECT id, customer_name, quote, context, published_at, created_at FROM testimonials
		WHERE status='published' AND consent ORDER BY published_at DESC NULLS LAST LIMIT 24`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Testimonial{}
	for rows.Next() {
		var t Testimonial
		if err := rows.Scan(&t.ID, &t.CustomerName, &t.Quote, &t.Context, &t.PublishedAt, &t.CreatedAt); err != nil {
			return err
		}
		out = append(out, t)
	}
	w.Header().Set("Cache-Control", "public, max-age=60, stale-while-revalidate=600")
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

func (h Handler) OwnerTestimonials(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(), `SELECT id, customer_name, quote, context, source, consent, status, published_at, created_at
		FROM testimonials ORDER BY created_at DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Testimonial{}
	for rows.Next() {
		var t Testimonial
		if err := rows.Scan(&t.ID, &t.CustomerName, &t.Quote, &t.Context, &t.Source, &t.Consent, &t.Status, &t.PublishedAt, &t.CreatedAt); err != nil {
			return err
		}
		out = append(out, t)
	}
	httpx.JSON(w, http.StatusOK, out)
	return rows.Err()
}

func (h Handler) OwnerSaveTestimonial(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		CustomerName string `json:"customerName"`
		Quote        string `json:"quote"`
		Context      string `json:"context"`
		Source       string `json:"source"`
		Consent      bool   `json:"consent"`
		Status       string `json:"status"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	if strings.TrimSpace(in.CustomerName) == "" {
		f.Add("customerName", "Enter the customer's name as they agreed to be shown.")
	}
	if q := strings.TrimSpace(in.Quote); q == "" || len(q) > 1200 {
		f.Add("quote", "Enter the customer's words (up to 1200 characters).")
	}
	if in.Status != "draft" && in.Status != "published" {
		f.Add("status", "Choose draft or published.")
	}
	if in.Status == "published" && !in.Consent {
		f.Add("consent", "Only publish a testimonial the customer agreed to share.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	if in.Source == "" {
		in.Source = "direct"
	}
	ctx := r.Context()
	idParam := chi.URLParam(r, "id")
	return db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var id uuid.UUID
		var err error
		if idParam == "" {
			err = tx.QueryRow(ctx, `INSERT INTO testimonials (customer_name, quote, context, source, consent, status, published_at, created_by)
				VALUES ($1,$2,$3,$4,$5,$6, CASE WHEN $6='published' THEN now() END, $7) RETURNING id`,
				strings.TrimSpace(in.CustomerName), strings.TrimSpace(in.Quote), in.Context, in.Source, in.Consent, in.Status, auth.ActorID(ctx)).Scan(&id)
		} else {
			if id, err = uuid.Parse(idParam); err != nil {
				return httpx.NotFound("Testimonial not found.")
			}
			_, err = tx.Exec(ctx, `UPDATE testimonials SET customer_name=$2, quote=$3, context=$4, source=$5, consent=$6, status=$7,
				published_at = CASE WHEN $7='published' THEN coalesce(published_at, now()) ELSE NULL END WHERE id=$1`,
				id, strings.TrimSpace(in.CustomerName), strings.TrimSpace(in.Quote), in.Context, in.Source, in.Consent, in.Status)
		}
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{Action: "testimonials.save", ObjectType: "testimonial", ObjectID: id.String(), After: in}); err != nil {
			return err
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"id": id})
		return nil
	})
}

func (h Handler) OwnerDeleteTestimonial(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	if _, err := h.Pool.Exec(ctx, `DELETE FROM testimonials WHERE id=$1`, id); err != nil {
		return err
	}
	_ = audit.Write(ctx, h.Pool, audit.Entry{Action: "testimonials.delete", ObjectType: "testimonial", ObjectID: id.String()})
	w.WriteHeader(http.StatusNoContent)
	return nil
}
