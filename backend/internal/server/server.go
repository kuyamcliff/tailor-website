// Package server wires configuration, services and HTTP routes, and runs background jobs.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kuyamcliff/tailor-website/backend/internal/analytics"
	"github.com/kuyamcliff/tailor-website/backend/internal/appointments"
	"github.com/kuyamcliff/tailor-website/backend/internal/audit"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth"
	"github.com/kuyamcliff/tailor-website/backend/internal/auth/authapi"
	"github.com/kuyamcliff/tailor-website/backend/internal/catalog"
	"github.com/kuyamcliff/tailor-website/backend/internal/config"
	"github.com/kuyamcliff/tailor-website/backend/internal/customers"
	"github.com/kuyamcliff/tailor-website/backend/internal/designs"
	"github.com/kuyamcliff/tailor-website/backend/internal/fabrics"
	"github.com/kuyamcliff/tailor-website/backend/internal/garments"
	"github.com/kuyamcliff/tailor-website/backend/internal/measurements"
	"github.com/kuyamcliff/tailor-website/backend/internal/notifications"
	"github.com/kuyamcliff/tailor-website/backend/internal/orders"
	"github.com/kuyamcliff/tailor-website/backend/internal/payments"
	"github.com/kuyamcliff/tailor-website/backend/internal/platform/httpx"
	"github.com/kuyamcliff/tailor-website/backend/internal/portfolio"
	"github.com/kuyamcliff/tailor-website/backend/internal/quotes"
	"github.com/kuyamcliff/tailor-website/backend/internal/references"
	"github.com/kuyamcliff/tailor-website/backend/internal/settings"
	"github.com/kuyamcliff/tailor-website/backend/internal/staff"
	"github.com/kuyamcliff/tailor-website/backend/internal/support"
	"github.com/kuyamcliff/tailor-website/backend/internal/uploads"
)

type App struct {
	Router        http.Handler
	Pool          *pgxpool.Pool
	Log           *slog.Logger
	Payments      *payments.Service
	Notifications *notifications.Service
	Uploads       *uploads.Service
}

type H = httpx.Handler

func New(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) (*App, error) {
	store, err := uploads.NewStorage(cfg.Storage)
	if err != nil {
		return nil, err
	}
	metrics := httpx.NewMetrics()
	limiter := httpx.NewRateLimiter()
	limiter.Multiplier = cfg.RateLimitMultiplier
	st := settings.NewService(pool)
	sessions := &auth.Sessions{Pool: pool, TTL: cfg.SessionTTL, CookieSecure: cfg.CookieSecure, CookieDomain: cfg.CookieDomain}
	pay := payments.NewService(cfg, pool, st, log, metrics)
	notif := notifications.NewService(pool, cfg, log)
	up := &uploads.Service{Pool: pool, Store: store, Settings: st, Log: log}
	refs := references.Handler{Pool: pool, Store: store, Settings: st, Provider: references.New(cfg)}

	authH := authapi.Handler{Pool: pool, Sessions: sessions, Settings: st}
	settingsH := settings.Handler{Svc: st}
	measH := measurements.Handler{Pool: pool}
	garH := garments.Handler{Pool: pool}
	assetH := garments.AssetsHandler{Handler: garH, Store: store}
	fabH := fabrics.Handler{Pool: pool}
	catH := catalog.Handler{Pool: pool}
	portH := portfolio.Handler{Pool: pool}
	desH := designs.Handler{Pool: pool, Settings: st}
	custH := customers.Handler{Pool: pool}
	ordH := orders.Handler{Pool: pool, Settings: st}
	qH := quotes.Handler{Pool: pool, Settings: st}
	apptH := appointments.Handler{Pool: pool, Settings: st}
	supH := support.Handler{Pool: pool, Settings: st}
	anaH := analytics.Handler{Pool: pool, Settings: st}
	staffH := staff.Handler{Pool: pool, Sessions: sessions}
	notifH := notifications.Handler{Pool: pool}
	auditH := audit.Handler{Pool: pool}

	blocker := func(ctx context.Context, key string) string {
		if reason := pay.FlagBlocker(ctx, key); reason != "" {
			return reason
		}
		return refs.Blocker(ctx, key)
	}

	r := chi.NewRouter()
	r.Use(httpx.RequestContext(log, cfg.TrustedProxies))
	r.Use(httpx.AccessLog(metrics))
	r.Use(httpx.Recover)
	r.Use(httpx.SecurityHeaders(cfg.IsProduction()))
	r.Use(httpx.CORS(cfg.AllowedOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	// Metrics are only served on the internal network: require the internal key when one is set.
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if cfg.InternalAPIKey != "" && r.Header.Get("X-Internal-Key") != cfg.InternalAPIKey {
			http.NotFound(w, r)
			return
		}
		metrics.ServeHTTP(w, r)
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(sessions.Authenticate)
		r.Use(limiter.Limit("global", 600, time.Minute))

		// Public configuration and content
		r.Method(http.MethodGet, "/config", H(settingsH.PublicConfig))
		r.Method(http.MethodGet, "/content", H(settingsH.PublicContent))

		// Auth
		r.Route("/auth", func(r chi.Router) {
			r.Method(http.MethodGet, "/me", H(authH.Me))
			r.With(limiter.Limit("auth", 10, time.Minute)).Method(http.MethodPost, "/signup", H(authH.SignUp))
			r.With(limiter.Limit("auth", 10, time.Minute)).Method(http.MethodPost, "/signin", H(authH.SignIn))
			r.Method(http.MethodPost, "/signout", H(authH.SignOut))
			r.With(limiter.Limit("reset", 5, 15*time.Minute)).Method(http.MethodPost, "/password-reset", H(authH.RequestPasswordReset))
			r.With(limiter.Limit("reset", 5, 15*time.Minute)).Method(http.MethodPost, "/password-reset/confirm", H(authH.ConfirmPasswordReset))
			r.Group(func(r chi.Router) {
				r.Use(auth.RequireUser)
				r.Method(http.MethodPost, "/password", H(authH.ChangePassword))
				r.Method(http.MethodGet, "/sessions", H(authH.ListSessions))
				r.Method(http.MethodPost, "/sessions/revoke-others", H(authH.RevokeOtherSessions))
			})
		})

		// Catalog and studio
		r.Method(http.MethodGet, "/products", H(catH.PublicList))
		r.Method(http.MethodGet, "/products/facets", H(catH.Facets))
		r.Method(http.MethodGet, "/products/{slug}", H(catH.PublicGet))
		r.Method(http.MethodGet, "/fabrics", H(fabH.PublicList))
		r.Method(http.MethodGet, "/fabrics/{key}", H(fabH.PublicGet))
		r.Method(http.MethodGet, "/garments", H(garH.PublicList))
		r.Method(http.MethodGet, "/garments/{key}/studio", H(garH.PublicStudio))
		r.Method(http.MethodGet, "/assets/{key}/versions/{version}", H(garH.PublicAssetVersion))
		r.Method(http.MethodGet, "/asset-files/{name}", H(assetH.ServeFile))
		r.Method(http.MethodGet, "/measurements/fields", H(measH.ListFields))
		r.Method(http.MethodPost, "/measurements/validate", H(measH.ValidateEndpoint))
		r.Method(http.MethodGet, "/portfolio", H(portH.PublicList))
		r.Method(http.MethodGet, "/portfolio/{slug}", H(portH.PublicGet))
		r.Method(http.MethodGet, "/testimonials", H(portH.PublicTestimonials))

		// Designs (guests via X-Guest-Token, or signed-in customers)
		r.Method(http.MethodPost, "/designs/evaluate", H(desH.Evaluate))
		r.Method(http.MethodGet, "/designs", H(desH.MyDesigns))
		r.With(limiter.Limit("designs", 60, 10*time.Minute)).Method(http.MethodPost, "/designs", H(desH.Save))
		r.Method(http.MethodGet, "/designs/{id}", H(desH.Get))
		r.Method(http.MethodPut, "/designs/{id}", H(desH.Save))
		r.Method(http.MethodDelete, "/designs/{id}", H(desH.Delete))
		r.Method(http.MethodGet, "/design-versions/{versionId}", H(desH.GetVersion))

		// Uploads
		r.With(limiter.Limit("uploads", 60, 10*time.Minute)).Method(http.MethodPost, "/uploads", H(up.Create))
		r.Method(http.MethodGet, "/uploads/{id}", H(up.Serve))
		r.Method(http.MethodGet, "/uploads/{id}/{variant}", H(up.Serve))
		r.Method(http.MethodDelete, "/uploads/{id}", H(up.Delete))
		r.With(limiter.Limit("analysis", 20, 10*time.Minute)).Method(http.MethodPost, "/uploads/{id}/analysis", H(refs.Analyze))

		// Bespoke requests and quotes
		r.With(limiter.Limit("submit", 20, 10*time.Minute)).Method(http.MethodPost, "/requests", H(qH.Submit))
		r.Method(http.MethodGet, "/requests/{id}", H(qH.CustomerGet))
		r.With(limiter.Limit("submit", 20, 10*time.Minute)).Method(http.MethodPost, "/requests/{id}/reply", H(qH.CustomerReply))
		r.Method(http.MethodGet, "/quotes/{id}", H(qH.CustomerGetQuote))
		r.Method(http.MethodPost, "/quotes/{id}/accept", H(qH.Accept))
		r.Method(http.MethodPost, "/quotes/{id}/decline", H(qH.Decline))
		r.Method(http.MethodPost, "/quotes/{id}/request-changes", H(qH.RequestChanges))

		// Orders and payments
		r.With(limiter.Limit("submit", 20, 10*time.Minute)).Method(http.MethodPost, "/checkout", H(ordH.Checkout))
		r.Method(http.MethodGet, "/orders/{id}", H(ordH.CustomerGet))
		r.Method(http.MethodGet, "/orders/{id}/measurements", H(ordH.CustomerMeasurements))
		r.Method(http.MethodGet, "/payments/methods", H(pay.Methods))
		r.With(limiter.Limit("payments", 20, 10*time.Minute)).Method(http.MethodPost, "/payments/intent", H(pay.Intent))
		r.Method(http.MethodGet, "/payments/{id}", H(pay.Get))
		r.With(limiter.Limit("webhooks", 300, time.Minute)).Method(http.MethodPost, "/payments/{provider}/webhook", H(pay.Webhook))
		r.With(limiter.Limit("webhooks", 300, time.Minute)).Method(http.MethodPut, "/payments/{provider}/webhook", H(pay.Webhook))

		// Appointments
		r.Group(func(r chi.Router) {
			r.Use(st.RequireFlag("appointments"))
			r.Method(http.MethodGet, "/appointments/slots", H(apptH.PublicSlots))
			r.With(limiter.Limit("submit", 20, 10*time.Minute)).Method(http.MethodPost, "/appointments", H(apptH.Book))
			r.Method(http.MethodGet, "/appointments/{id}", H(apptH.CustomerGet))
			r.Method(http.MethodPost, "/appointments/{id}/change", H(apptH.CustomerChange))
		})

		// Support
		r.Group(func(r chi.Router) {
			r.Use(st.RequireFlag("support_inbox"))
			r.With(limiter.Limit("submit", 20, 10*time.Minute)).Method(http.MethodPost, "/support", H(supH.Create))
			r.Method(http.MethodGet, "/support/{id}", H(supH.CustomerGet))
			r.With(limiter.Limit("submit", 30, 10*time.Minute)).Method(http.MethodPost, "/support/{id}/reply", H(supH.CustomerReply))
		})

		// Signed-in customer area
		r.Route("/me", func(r chi.Router) {
			r.Use(auth.RequireUser)
			r.Method(http.MethodGet, "/profile", H(custH.GetProfile))
			r.Method(http.MethodPut, "/profile", H(custH.UpdateProfile))
			r.Method(http.MethodPost, "/claim-guest", H(custH.ClaimGuestData))
			r.Method(http.MethodGet, "/addresses", H(custH.ListAddresses))
			r.Method(http.MethodPost, "/addresses", H(custH.SaveAddress))
			r.Method(http.MethodPut, "/addresses/{id}", H(custH.SaveAddress))
			r.Method(http.MethodDelete, "/addresses/{id}", H(custH.DeleteAddress))
			r.Method(http.MethodGet, "/wishlist", H(custH.Wishlist))
			r.Method(http.MethodPost, "/wishlist", H(custH.AddWishlist))
			r.Method(http.MethodDelete, "/wishlist/{id}", H(custH.RemoveWishlist))
			r.Method(http.MethodGet, "/export", H(custH.Export))
			r.Post("/delete-account", custH.DeleteAccount(store))
			r.Method(http.MethodGet, "/measurement-profiles", H(measH.MyProfiles))
			r.Method(http.MethodPost, "/measurement-profiles", H(measH.CreateProfile))
			r.Method(http.MethodPut, "/measurement-profiles/{id}", H(measH.UpdateProfile))
			r.Method(http.MethodDelete, "/measurement-profiles/{id}", H(measH.DeleteProfile))
			r.Method(http.MethodGet, "/measurement-profiles/{id}/versions", H(measH.ProfileVersions))
			r.Method(http.MethodPost, "/measurement-profiles/{id}/versions", H(measH.AddVersion))
			r.Method(http.MethodGet, "/orders", H(ordH.MyOrders))
			r.Method(http.MethodGet, "/requests", H(qH.MyRequests))
			r.Method(http.MethodGet, "/appointments", H(apptH.MyAppointments))
			r.Method(http.MethodGet, "/support", H(supH.MyThreads))
			r.Method(http.MethodGet, "/notifications", H(notifH.List))
			r.Method(http.MethodPost, "/notifications/read", H(notifH.MarkRead))
		})

		// Owner dashboard
		r.Route("/owner", func(r chi.Router) {
			r.Use(auth.RequireStaff)
			perm := auth.RequirePermission
			r.Method(http.MethodGet, "/dashboard", H(anaH.Dashboard))
			r.Method(http.MethodGet, "/team", H(staffH.Team))
			r.With(perm("analytics.read")).Method(http.MethodGet, "/analytics", H(anaH.Metrics))
			r.With(perm("audit.read")).Method(http.MethodGet, "/audit", H(auditH.List))
			r.Method(http.MethodGet, "/notifications", H(notifH.List))
			r.Method(http.MethodPost, "/notifications/read", H(notifH.MarkRead))

			r.With(perm("requests.read")).Method(http.MethodGet, "/requests", H(qH.OwnerList))
			r.With(perm("requests.read")).Method(http.MethodGet, "/requests/{id}", H(qH.OwnerGet))
			r.With(perm("requests.update")).Method(http.MethodPatch, "/requests/{id}", H(qH.OwnerUpdate))
			r.With(perm("measurements.write")).Method(http.MethodPost, "/requests/{id}/verify-measurements", H(qH.OwnerVerifyMeasurements))
			r.With(perm("quotes.write")).Method(http.MethodPost, "/requests/{id}/quotes", H(qH.OwnerCreateQuote))
			r.With(perm("quotes.write")).Method(http.MethodGet, "/quotes", H(qH.OwnerListQuotes))
			r.With(perm("quotes.write")).Method(http.MethodGet, "/quotes/{id}", H(qH.OwnerGetQuote))
			r.With(perm("quotes.write")).Method(http.MethodPut, "/quotes/{id}", H(qH.OwnerReviseQuote))
			r.With(perm("quotes.write")).Method(http.MethodPost, "/quotes/{id}/send", H(qH.OwnerSendQuote))
			r.With(perm("quotes.write")).Method(http.MethodPost, "/quotes/{id}/withdraw", H(qH.OwnerWithdrawQuote))

			r.With(perm("orders.read")).Method(http.MethodGet, "/orders", H(ordH.OwnerList))
			r.With(perm("orders.read")).Method(http.MethodGet, "/orders/{id}", H(ordH.OwnerGet))
			r.With(perm("orders.update")).Method(http.MethodPost, "/orders/{id}/status", H(ordH.OwnerTransition))
			r.With(perm("orders.update")).Method(http.MethodPost, "/orders/{id}/notes", H(ordH.OwnerAddNote))
			r.With(perm("orders.update")).Method(http.MethodPatch, "/orders/{id}", H(ordH.OwnerUpdateDetails))
			r.With(perm("orders.update")).Method(http.MethodPost, "/orders/{id}/tasks", H(ordH.OwnerSaveTask))
			r.With(perm("orders.update")).Method(http.MethodPost, "/orders/{id}/fittings", H(ordH.OwnerAddFitting))
			r.With(perm("orders.update"), perm("payments.read")).Method(http.MethodPost, "/orders/{id}/payments/manual", H(pay.OwnerRecordManual))

			r.With(perm("payments.read")).Method(http.MethodGet, "/payments", H(pay.OwnerList))
			r.With(perm("payments.read")).Method(http.MethodPost, "/payments/{id}/recheck", H(pay.OwnerRecheck))
			r.With(perm("payments.refund")).Method(http.MethodPost, "/payments/{id}/refunds", H(pay.OwnerRefund))

			r.With(perm("customers.read")).Method(http.MethodGet, "/customers", H(custH.OwnerList))
			r.With(perm("customers.read")).Method(http.MethodGet, "/customers/{id}", H(custH.OwnerGet))
			r.With(perm("customers.update")).Method(http.MethodPatch, "/customers/{id}", H(custH.OwnerUpdate))
			r.With(perm("customers.read")).Method(http.MethodGet, "/customers/{id}/measurement-profiles", H(measH.CustomerProfiles))
			r.With(perm("measurements.write")).Method(http.MethodPost, "/customers/{id}/measurement-profiles", H(measH.StaffCreateProfile))
			r.With(perm("customers.read")).Method(http.MethodGet, "/measurement-profiles/{id}/versions", H(measH.StaffProfileVersions))
			r.With(perm("measurements.write")).Method(http.MethodPost, "/measurement-profiles/{id}/versions", H(measH.StaffAddVersion))
			r.With(perm("customers.read")).Method(http.MethodGet, "/measurement-versions/{id}", H(measH.StaffVersion))

			r.With(perm("appointments.read")).Method(http.MethodGet, "/appointments", H(apptH.OwnerList))
			r.With(perm("appointments.write")).Method(http.MethodPost, "/appointments", H(apptH.OwnerCreate))
			r.With(perm("appointments.write")).Method(http.MethodPatch, "/appointments/{id}", H(apptH.OwnerUpdate))
			r.With(perm("appointments.write")).Method(http.MethodPost, "/appointments/{id}/change", H(apptH.OwnerChange))
			r.With(perm("appointments.read")).Method(http.MethodGet, "/availability", H(apptH.OwnerAvailability))
			r.With(perm("appointments.write")).Method(http.MethodPut, "/availability/rules", H(apptH.OwnerPutRules))
			r.With(perm("appointments.write")).Method(http.MethodPost, "/availability/blocks", H(apptH.OwnerAddBlock))
			r.With(perm("appointments.write")).Method(http.MethodDelete, "/availability/blocks/{id}", H(apptH.OwnerDeleteBlock))

			r.With(perm("support.read")).Method(http.MethodGet, "/support", H(supH.OwnerList))
			r.With(perm("support.read")).Method(http.MethodGet, "/support/{id}", H(supH.OwnerGet))
			r.With(perm("support.write")).Method(http.MethodPost, "/support/{id}/reply", H(supH.OwnerReply))
			r.With(perm("support.write")).Method(http.MethodPatch, "/support/{id}", H(supH.OwnerUpdate))

			r.With(perm("products.write")).Method(http.MethodGet, "/products", H(catH.OwnerList))
			r.With(perm("products.write")).Method(http.MethodPost, "/products", H(catH.OwnerSave))
			r.With(perm("products.write")).Method(http.MethodGet, "/products/{id}", H(catH.OwnerGet))
			r.With(perm("products.write")).Method(http.MethodPut, "/products/{id}", H(catH.OwnerSave))
			r.With(perm("products.write")).Method(http.MethodGet, "/categories", H(catH.OwnerCategories))
			r.With(perm("products.write")).Method(http.MethodPost, "/categories", H(catH.OwnerUpsertCategory))

			r.With(perm("fabrics.write")).Method(http.MethodGet, "/fabrics", H(fabH.OwnerList))
			r.With(perm("fabrics.write")).Method(http.MethodPost, "/fabrics", H(fabH.OwnerUpsert))

			r.With(perm("garments.write")).Method(http.MethodGet, "/garments", H(garH.OwnerList))
			r.With(perm("garments.write")).Method(http.MethodPost, "/garments", H(garH.OwnerUpsertType))
			r.With(perm("garments.write")).Method(http.MethodGet, "/garments/{key}", H(garH.OwnerGet))
			r.With(perm("garments.write")).Method(http.MethodPost, "/garments/{key}/groups", H(garH.OwnerUpsertGroup))
			r.With(perm("garments.write")).Method(http.MethodPost, "/option-groups/{groupId}/values", H(garH.OwnerUpsertValue))
			r.With(perm("fit_rules.write")).Method(http.MethodPut, "/garments/{key}/fit-rules", H(garH.OwnerPutFitRules))
			r.With(perm("fit_rules.write")).Method(http.MethodPut, "/garments/{key}/measurement-fields", H(garH.OwnerPutMeasurementFields))
			r.With(perm("fit_rules.write")).Method(http.MethodPost, "/measurement-fields", H(garH.OwnerUpsertMeasurementField))
			r.With(perm("garments.write")).Method(http.MethodPut, "/garments/{key}/sizes", H(garH.OwnerPutSizes))

			r.With(perm("assets.write")).Method(http.MethodGet, "/assets", H(assetH.List))
			r.With(perm("assets.write")).Method(http.MethodPost, "/assets/files", H(assetH.UploadFile))
			r.With(perm("assets.write")).Method(http.MethodPost, "/assets", H(assetH.CreateManifest))
			r.With(perm("assets.write")).Method(http.MethodPost, "/assets/{id}/status", H(assetH.Transition))

			r.With(perm("portfolio.write")).Method(http.MethodGet, "/portfolio", H(portH.OwnerList))
			r.With(perm("portfolio.write")).Method(http.MethodPost, "/portfolio", H(portH.OwnerSave))
			r.With(perm("portfolio.write")).Method(http.MethodPut, "/portfolio/{id}", H(portH.OwnerSave))
			r.With(perm("portfolio.write")).Method(http.MethodDelete, "/portfolio/{id}", H(portH.OwnerDelete))
			r.With(perm("testimonials.write")).Method(http.MethodGet, "/testimonials", H(portH.OwnerTestimonials))
			r.With(perm("testimonials.write")).Method(http.MethodPost, "/testimonials", H(portH.OwnerSaveTestimonial))
			r.With(perm("testimonials.write")).Method(http.MethodPut, "/testimonials/{id}", H(portH.OwnerSaveTestimonial))
			r.With(perm("testimonials.write")).Method(http.MethodDelete, "/testimonials/{id}", H(portH.OwnerDeleteTestimonial))

			r.With(perm("content.write")).Method(http.MethodGet, "/content", H(settingsH.ListContent))
			r.With(perm("content.write")).Method(http.MethodPut, "/content/{key}", H(settingsH.PutContent))
			r.With(perm("settings.write")).Method(http.MethodGet, "/settings/business", H(settingsH.GetBusiness))
			r.With(perm("settings.write")).Method(http.MethodPut, "/settings/business", H(settingsH.PutBusiness))
			r.With(perm("settings.write")).Get("/flags", settingsH.ListFlags(blocker))
			r.With(perm("settings.write")).Put("/flags/{key}", settingsH.SetFlag(blocker))
			r.With(perm("staff.write")).Method(http.MethodGet, "/staff", H(staffH.List))
			r.With(perm("staff.write")).Method(http.MethodPost, "/staff", H(staffH.Create))
			r.With(perm("staff.write")).Method(http.MethodPut, "/staff/{id}", H(staffH.Update))
		})
	})

	return &App{Router: r, Pool: pool, Log: log, Payments: pay, Notifications: notif, Uploads: up}, nil
}

// StartBackground runs periodic jobs until ctx is cancelled. Each job is safe to run on several
// instances at once (row locks and idempotent updates).
func (a *App) StartBackground(ctx context.Context) {
	go a.Notifications.Run(ctx)
	every := func(d time.Duration, name string, fn func(context.Context)) {
		go func() {
			t := time.NewTicker(d)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					jobCtx, cancel := context.WithTimeout(ctx, d)
					func() {
						defer func() {
							if p := recover(); p != nil {
								a.Log.Error("background job panic", "job", name, "panic", p)
							}
						}()
						fn(jobCtx)
					}()
					cancel()
				}
			}
		}()
	}
	every(15*time.Second, "payments.reconcile", a.Payments.Reconcile)
	every(time.Minute, "quotes.expire", func(ctx context.Context) {
		if n, err := quotes.ExpireDue(ctx, a.Pool); err != nil {
			a.Log.Error("expire quotes", "error", err)
		} else if n > 0 {
			a.Log.Info("quotes expired", "count", n)
		}
	})
	every(5*time.Minute, "appointments.reminders", func(ctx context.Context) {
		if err := appointments.SendReminders(ctx, a.Pool); err != nil {
			a.Log.Error("appointment reminders", "error", err)
		}
	})
	every(time.Hour, "uploads.retention", func(ctx context.Context) {
		if n, err := a.Uploads.PurgeExpired(ctx); err != nil {
			a.Log.Error("upload retention", "error", err)
		} else if n > 0 {
			a.Log.Info("uploads purged by retention policy", "count", n)
		}
	})
	every(time.Hour, "sessions.cleanup", func(ctx context.Context) {
		_, _ = a.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now() - interval '7 days' OR revoked_at < now() - interval '7 days'`)
	})
}
