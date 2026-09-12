package app

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/booking"
	"github.com/thvnhtai/gearshare/internal/user"
)

//go:embed templates/admin/*.html
var adminTemplatesFS embed.FS

var adminTemplates = template.Must(template.ParseFS(adminTemplatesFS, "templates/admin/*.html"))

// AdminHandler is the cookie-session + CSRF auth style's home
// (docs/architecture.md): a classic server-rendered surface using
// html/template, distinct from the JSON API's JWT bearer tokens. html/template
// (not text/template) is used specifically for its automatic contextual
// escaping — booking data interpolated into the table can't become an XSS
// vector even though none of it is currently user-suppliable free text.
type AdminHandler struct {
	sessions *auth.SessionManager
	users    *user.Repository
	hasher   user.PasswordHasher
	bookings *booking.Repository
	secureCookies bool
}

func NewAdminHandler(sessions *auth.SessionManager, users *user.Repository, hasher user.PasswordHasher, bookings *booking.Repository, secureCookies bool) *AdminHandler {
	return &AdminHandler{sessions: sessions, users: users, hasher: hasher, bookings: bookings, secureCookies: secureCookies}
}

func (h *AdminHandler) LoginForm(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GenerateCSRFToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	auth.IssueCSRFCookie(w, token, h.secureCookies)
	_ = adminTemplates.ExecuteTemplate(w, "login.html", map[string]string{"CSRFToken": token})
}

func (h *AdminHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	email := r.FormValue("email")
	password := r.FormValue("password")

	u, err := h.users.GetByEmail(r.Context(), email)
	loginFailed := errors.Is(err, user.ErrNotFound) || (err == nil && !h.hasher.Verify(password, u.PasswordHash))
	if err != nil && !errors.Is(err, user.ErrNotFound) {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if loginFailed || u.Role != user.RoleAdmin {
		token, _ := auth.GenerateCSRFToken()
		auth.IssueCSRFCookie(w, token, h.secureCookies)
		w.WriteHeader(http.StatusUnauthorized)
		_ = adminTemplates.ExecuteTemplate(w, "login.html", map[string]string{
			"Error": "Invalid credentials or not an admin account", "CSRFToken": token,
		})
		return
	}

	h.sessions.IssueCookie(w, u.ID, h.secureCookies)
	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

func (h *AdminHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.sessions.ClearCookie(w)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

type dashboardBookingRow struct {
	ID                int64
	ListingID         int64
	RenterID          int64
	Status            booking.Status
	TotalPriceDollars string
}

func (h *AdminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	bookings, err := h.bookings.ListRecent(r.Context(), 50)
	if err != nil {
		http.Error(w, "could not load bookings", http.StatusInternalServerError)
		return
	}

	rows := make([]dashboardBookingRow, 0, len(bookings))
	for _, b := range bookings {
		rows = append(rows, dashboardBookingRow{
			ID: b.ID, ListingID: b.ListingID, RenterID: b.RenterID, Status: b.Status,
			TotalPriceDollars: formatCents(b.TotalPriceCents),
		})
	}

	token, err := auth.GenerateCSRFToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	auth.IssueCSRFCookie(w, token, h.secureCookies)

	_ = adminTemplates.ExecuteTemplate(w, "dashboard.html", map[string]interface{}{
		"Bookings":  rows,
		"CSRFToken": token,
	})
}

func formatCents(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100)
}
