// Package audit records append-only audit entries and status histories.
package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/db"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
)

type Entry struct {
	Action     string
	ObjectType string
	ObjectID   string
	Before     any
	After      any
}

// Write appends an audit entry using the request's principal and request ID.
// It runs on q so it commits or rolls back with the surrounding transaction.
func Write(ctx context.Context, q db.Querier, e Entry) error {
	var role *string
	if p := auth.FromContext(ctx); p != nil {
		role = &p.Role
	}
	_, err := q.Exec(ctx, `INSERT INTO audit_logs (actor_id, actor_role, action, object_type, object_id, before, after, request_id, ip)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		auth.ActorID(ctx), role, e.Action, e.ObjectType, nullable(e.ObjectID),
		jsonOrNil(e.Before), jsonOrNil(e.After), nullable(httpx.RequestIDFrom(ctx)), nullable(httpx.ClientIP(ctx)))
	return err
}

// Status records a status transition in the shared status history table.
func Status(ctx context.Context, q db.Querier, objectType string, id uuid.UUID, oldStatus, newStatus, note string, customerVisible bool) error {
	var old *string
	if oldStatus != "" {
		old = &oldStatus
	}
	_, err := q.Exec(ctx, `INSERT INTO status_history (object_type, object_id, old_status, new_status, actor_id, actor_label, note, customer_visible)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		objectType, id, old, newStatus, auth.ActorID(ctx), auth.ActorLabel(ctx), nullable(note), customerVisible)
	return err
}

type HistoryItem struct {
	OldStatus *string   `json:"oldStatus"`
	NewStatus string    `json:"newStatus"`
	Actor     string    `json:"actor"`
	Note      *string   `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

// History returns the status history for an object; customerView hides internal entries.
func History(ctx context.Context, q db.Querier, objectType string, id uuid.UUID, customerView bool) ([]HistoryItem, error) {
	rows, err := q.Query(ctx, `SELECT old_status, new_status, actor_label, note, created_at FROM status_history
		WHERE object_type=$1 AND object_id=$2 AND ($3 = false OR customer_visible) ORDER BY created_at`, objectType, id, customerView)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HistoryItem{}
	for rows.Next() {
		var h HistoryItem
		if err := rows.Scan(&h.OldStatus, &h.NewStatus, &h.Actor, &h.Note, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

type LogItem struct {
	ID         uuid.UUID       `json:"id"`
	ActorID    *uuid.UUID      `json:"actorId"`
	ActorName  *string         `json:"actorName"`
	ActorRole  *string         `json:"actorRole"`
	Action     string          `json:"action"`
	ObjectType string          `json:"objectType"`
	ObjectID   *string         `json:"objectId"`
	Before     json.RawMessage `json:"before"`
	After      json.RawMessage `json:"after"`
	RequestID  *string         `json:"requestId"`
	CreatedAt  time.Time       `json:"createdAt"`
}

// Handler serves the owner audit log with filters.
type Handler struct{ Pool *pgxpool.Pool }

func (h Handler) List(w http.ResponseWriter, r *http.Request) error {
	page := httpx.ParsePage(r, 50, 200)
	qs := r.URL.Query()
	objType, objID, action := qs.Get("objectType"), qs.Get("objectId"), qs.Get("action")
	rows, err := h.Pool.Query(r.Context(), `SELECT a.id, a.actor_id, u.full_name, a.actor_role, a.action, a.object_type, a.object_id,
		a.before, a.after, a.request_id, a.created_at, count(*) OVER()
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE ($1 = '' OR a.object_type = $1) AND ($2 = '' OR a.object_id = $2) AND ($3 = '' OR a.action LIKE $3 || '%')
		ORDER BY a.created_at DESC LIMIT $4 OFFSET $5`, objType, objID, action, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	defer rows.Close()
	var items []LogItem
	total := 0
	for rows.Next() {
		var it LogItem
		var before, after []byte
		if err := rows.Scan(&it.ID, &it.ActorID, &it.ActorName, &it.ActorRole, &it.Action, &it.ObjectType, &it.ObjectID,
			&before, &after, &it.RequestID, &it.CreatedAt, &total); err != nil {
			return err
		}
		it.Before, it.After = before, after
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	httpx.JSON(w, http.StatusOK, httpx.NewList(items, total, page))
	return nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func jsonOrNil(v any) []byte {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
