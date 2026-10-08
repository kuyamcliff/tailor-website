// Package staff lets the owner manage staff accounts and roles. Role changes are audited and
// the last active owner can never be demoted or disabled.
package staff

import (
	"net/http"
	"net/mail"
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

type Handler struct {
	Pool     *pgxpool.Pool
	Sessions *auth.Sessions
}

type Member struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Email       *string    `json:"email"`
	Role        string     `json:"role"`
	RoleName    string     `json:"roleName"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func (h Handler) List(w http.ResponseWriter, r *http.Request) error {
	rows, err := h.Pool.Query(r.Context(), `SELECT u.id, u.full_name, u.email, u.role, ro.name, u.status, u.last_login_at, u.created_at
		FROM users u JOIN roles ro ON ro.key=u.role WHERE ro.is_staff AND u.deleted_at IS NULL ORDER BY u.created_at`)
	if err != nil {
		return err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.Name, &m.Email, &m.Role, &m.RoleName, &m.Status, &m.LastLoginAt, &m.CreatedAt); err != nil {
			return err
		}
		out = append(out, m)
	}
	roles, err := h.Pool.Query(r.Context(), `SELECT key, name, permissions FROM roles WHERE is_staff ORDER BY key`)
	if err != nil {
		return err
	}
	defer roles.Close()
	type role struct {
		Key         string   `json:"key"`
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	rl := []role{}
	for roles.Next() {
		var x role
		if err := roles.Scan(&x.Key, &x.Name, &x.Permissions); err != nil {
			return err
		}
		rl = append(rl, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"staff": out, "roles": rl})
	return nil
}

func validRole(ctx interface{ Value(any) any }, role string) bool {
	switch role {
	case "owner", "manager", "tailor", "support", "content_manager":
		return true
	}
	return false
}

// Create adds a staff account with an initial password the person must change after first sign-in.
func (h Handler) Create(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f := httpx.Fields{}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Name == "" {
		f.Add("name", "Enter a name.")
	}
	if a, err := mail.ParseAddress(in.Email); err != nil || a.Address != in.Email {
		f.Add("email", "Enter a valid email.")
	}
	if !validRole(ctx, in.Role) {
		f.Add("role", "Choose a role.")
	}
	if err := auth.ValidatePassword(in.Password); err != nil {
		f.Add("password", "Use between 10 and 128 characters.")
	}
	if err := f.Err(); err != nil {
		return err
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	var id uuid.UUID
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO users (email, password_hash, full_name, role) VALUES ($1,$2,$3,$4) RETURNING id`,
			in.Email, hash, in.Name, in.Role).Scan(&id)
		if db.IsUniqueViolation(err, "") {
			return httpx.Validation(map[string]string{"email": "An account with this email already exists."})
		}
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "staff.create", ObjectType: "user", ObjectID: id.String(),
			After: map[string]string{"name": in.Name, "email": in.Email, "role": in.Role}})
	})
	if err != nil {
		return err
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{"id": id})
	return nil
}

// Update changes a staff member's role or status. Disabling revokes all their sessions immediately.
func (h Handler) Update(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Role   string `json:"role"`
		Status string `json:"status"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if !validRole(ctx, in.Role) || (in.Status != "active" && in.Status != "disabled") {
		return httpx.Validation(map[string]string{"role": "Choose a role and status."})
	}
	err = db.InTx(ctx, h.Pool, func(tx pgx.Tx) error {
		var oldRole, oldStatus string
		if err := tx.QueryRow(ctx, `SELECT role, status FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&oldRole, &oldStatus); err != nil {
			return err
		}
		if oldRole == "owner" && (in.Role != "owner" || in.Status != "active") {
			var owners int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='owner' AND status='active' AND deleted_at IS NULL`).Scan(&owners); err != nil {
				return err
			}
			if owners <= 1 {
				return httpx.Conflict("last_owner", "The last owner account cannot be demoted or disabled.")
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET role=$2, status=$3 WHERE id=$1`, id, in.Role, in.Status); err != nil {
			return err
		}
		if in.Status != "active" || in.Role != oldRole {
			if _, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{Action: "staff.role_change", ObjectType: "user", ObjectID: id.String(),
			Before: map[string]string{"role": oldRole, "status": oldStatus}, After: in})
	})
	if err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
