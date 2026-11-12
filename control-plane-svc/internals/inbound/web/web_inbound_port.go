// Package web is the operator console: a hypermedia (HATEOAS) HTML interface.
//
// Every page is rendered on the server and carries the links and forms for
// the transitions that are allowed right now. The rules live in one place
// (controlsFor): templates never decide what an operator may do, they only
// render the controls they are given. HTML forms only speak GET and POST, so
// deletion is "POST /users/{id}/delete" rather than DELETE.
package web

import (
	"bytes"
	"control-plane-svc/internals/domain"
	"control-plane-svc/internals/service"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

const usersPageSize = 50

type WebInboundPort struct {
	Address             string
	Port                int
	ControlPlaneService *service.ControlPlaneService
	pages               map[string]*template.Template
}

func NewWebInboundPort(address string, port int, controlPlaneService *service.ControlPlaneService) *WebInboundPort {
	return &WebInboundPort{
		Address:             address,
		Port:                port,
		ControlPlaneService: controlPlaneService,
		pages:               parsePages(),
	}
}

func parsePages() map[string]*template.Template {
	layout := template.Must(template.New("").ParseFS(templateFS, "templates/layout.html"))
	pages := make(map[string]*template.Template)
	for _, name := range []string{"users.html", "user.html", "new_user.html", "confirm_delete.html", "result.html", "error.html"} {
		pages[name] = template.Must(template.Must(layout.Clone()).ParseFS(templateFS, "templates/"+name))
	}
	return pages
}

func (wip *WebInboundPort) Routes() http.Handler {
	mux := http.NewServeMux()

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/users", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /users", wip.listUsers)
	mux.HandleFunc("GET /users/new", wip.newUserForm)
	mux.HandleFunc("POST /users", wip.createUser)
	mux.HandleFunc("GET /users/{id}", wip.showUser)
	mux.HandleFunc("GET /users/{id}/delete", wip.confirmDelete)
	mux.HandleFunc("POST /users/{id}/delete", wip.deleteUser)
	mux.HandleFunc("POST /users/{id}/password", wip.resetPassword)

	// CrossOriginProtection rejects cross-site form posts (CSRF).
	return securityHeaders(http.NewCrossOriginProtection().Handler(mux))
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		// Not "no-referrer": under that policy browsers send "Origin: null" on
		// form POSTs, and over plain HTTP (no Sec-Fetch-Site, since a .test
		// host is not a secure context) CrossOriginProtection then rejects
		// every same-origin POST with 403. "same-origin" keeps the real Origin.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

// ---- hypermedia ------------------------------------------------------------

// Control is a hypermedia affordance: a link (GET) or a form (POST).
type Control struct {
	Rel     string
	Label   string
	Method  string
	Href    string
	Danger  bool
	Primary bool
}

func userHref(id string) string { return "/users/" + url.PathEscape(id) }

// controlsFor is the single place that decides which transitions a user's
// state allows.
func controlsFor(user *domain.User) []Control {
	deleteControl := Control{Rel: "delete", Label: "Delete…", Method: http.MethodGet, Href: userHref(user.ID) + "/delete", Danger: true}
	switch user.State {
	case domain.CREATED:
		return []Control{
			{Rel: "reset-password", Label: "Reset temporary password", Method: http.MethodPost, Href: userHref(user.ID) + "/password"},
			deleteControl,
		}
	case domain.FAILED_CREATION, domain.FAILED_DELETION:
		return []Control{deleteControl}
	default: // PENDING_*: the request or the reconciler is still working on it
		return nil
	}
}

func stateHelp(state int) string {
	switch state {
	case domain.PENDING_CREATION:
		return "Creation in progress. If the request was interrupted, the reconciler finishes it within about a minute."
	case domain.CREATED:
		return "The account exists in Keycloak and its username and email are reserved."
	case domain.FAILED_CREATION:
		return "Keycloak rejected the creation. Delete this entry to free the username and email."
	case domain.PENDING_DELETION:
		return "Deletion in progress. If the request was interrupted, the reconciler finishes it within about a minute."
	case domain.FAILED_DELETION:
		return "Keycloak refused the deletion. Try deleting again; the details are in the service log."
	default:
		return "Unknown state."
	}
}

// UserView is what the templates see of a user.
type UserView struct {
	ID         string
	Href       string
	Username   string
	Email      string
	Realm      string
	KeycloakID string
	State      string
	StateClass string
	StateHelp  string
	InProgress bool
	Controls   []Control
}

func userView(user *domain.User) UserView {
	state := domain.StateToString(user.State)
	return UserView{
		ID:         user.ID,
		Href:       userHref(user.ID),
		Username:   user.Username,
		Email:      user.Email,
		Realm:      user.RealmAlias,
		KeycloakID: user.RealmID,
		State:      state,
		StateClass: strings.ToLower(strings.ReplaceAll(state, "_", "-")),
		StateHelp:  stateHelp(user.State),
		InProgress: user.State == domain.PENDING_CREATION || user.State == domain.PENDING_DELETION,
		Controls:   controlsFor(user),
	}
}

// ---- rendering -------------------------------------------------------------

func (wip *WebInboundPort) render(w http.ResponseWriter, r *http.Request, status int, page string, data any) {
	var buf bytes.Buffer
	if err := wip.pages[page].ExecuteTemplate(&buf, "layout", data); err != nil {
		slog.ErrorContext(r.Context(), "template rendering failed", "page", page, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

type errorPage struct {
	Title    string
	Status   int
	Message  string
	Controls []Control
}

func (wip *WebInboundPort) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	wip.render(w, r, status, "error.html", errorPage{
		Title:    http.StatusText(status),
		Status:   status,
		Message:  message,
		Controls: []Control{{Rel: "collection", Label: "Back to users", Method: http.MethodGet, Href: "/users"}},
	})
}

// loadUser reads the {id} path value and fetches the user, rendering 404 for
// malformed or unknown ids.
func (wip *WebInboundPort) loadUser(w http.ResponseWriter, r *http.Request) (*domain.User, bool) {
	id := r.PathValue("id")
	if !isUUID(id) {
		wip.renderError(w, r, http.StatusNotFound, "No user with this id.")
		return nil, false
	}
	user, err := wip.ControlPlaneService.GetUser(r.Context(), id)
	if errors.Is(err, service.ErrNotFound) {
		wip.renderError(w, r, http.StatusNotFound, "No user with this id. It may have been deleted.")
		return nil, false
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "loading user failed", "user_id", id, "error", err)
		wip.renderError(w, r, http.StatusInternalServerError, "Could not load the user.")
		return nil, false
	}
	return user, true
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}
	return true
}

// ---- users collection ------------------------------------------------------

type notice struct {
	Kind string
	Text string
}

// Notices shown after a redirect. Only fixed texts, so a crafted URL cannot
// inject a message.
var notices = map[string]notice{
	"deleted":          {"success", "User deleted. Its username and email are free again."},
	"deletion-pending": {"info", "Keycloak did not answer in time. The deletion is recorded and the reconciler will finish it."},
}

type usersPage struct {
	Title    string
	Notice   *notice
	Users    []UserView
	Controls []Control
	Prev     string
	Next     string
}

func (wip *WebInboundPort) listUsers(w http.ResponseWriter, r *http.Request) {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	users, err := wip.ControlPlaneService.ListUsers(r.Context(), (page-1)*usersPageSize, usersPageSize+1)
	if err != nil {
		slog.ErrorContext(r.Context(), "listing users failed", "error", err)
		wip.renderError(w, r, http.StatusInternalServerError, "Could not load the users.")
		return
	}

	data := usersPage{
		Title:    "Users",
		Controls: []Control{{Rel: "create-form", Label: "New user", Method: http.MethodGet, Href: "/users/new", Primary: true}},
	}
	if n, ok := notices[r.URL.Query().Get("notice")]; ok {
		data.Notice = &n
	}
	if len(users) > usersPageSize {
		users = users[:usersPageSize]
		data.Next = fmt.Sprintf("/users?page=%d", page+1)
	}
	if page > 1 {
		data.Prev = fmt.Sprintf("/users?page=%d", page-1)
	}
	for _, user := range users {
		data.Users = append(data.Users, userView(user))
	}
	wip.render(w, r, http.StatusOK, "users.html", data)
}

// ---- creation --------------------------------------------------------------

type newUserPage struct {
	Title    string
	Realms   []string
	Username string
	Email    string
	Realm    string
	Error    string
	Action   string
}

func (wip *WebInboundPort) newUserForm(w http.ResponseWriter, r *http.Request) {
	wip.render(w, r, http.StatusOK, "new_user.html", newUserPage{
		Title:  "New user",
		Realms: wip.ControlPlaneService.Realms(),
		Action: "/users",
	})
}

type resultPage struct {
	Title    string
	Kind     string
	Message  string
	Password string
	User     *UserView
	Controls []Control
}

func (wip *WebInboundPort) createUser(w http.ResponseWriter, r *http.Request) {
	form := newUserPage{
		Title:    "New user",
		Realms:   wip.ControlPlaneService.Realms(),
		Username: strings.TrimSpace(r.PostFormValue("username")),
		Email:    strings.TrimSpace(r.PostFormValue("email")),
		Realm:    r.PostFormValue("realm"),
		Action:   "/users",
	}
	if msg := validateNewUser(form); msg != "" {
		form.Error = msg
		wip.render(w, r, http.StatusUnprocessableEntity, "new_user.html", form)
		return
	}

	result, err := wip.ControlPlaneService.RegisterUser(r.Context(), form.Username, form.Email, form.Realm)
	switch {
	case err == nil:
		user := userView(result.User)
		message := "The account exists in Keycloak. Give the user this temporary password: Keycloak asks for a new one at the first login."
		if user.InProgress {
			message = "The account exists in Keycloak with this temporary password; the control plane will finish recording it shortly."
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Location", user.Href)
		wip.render(w, r, http.StatusCreated, "result.html", resultPage{
			Title:    "User created",
			Kind:     "success",
			Message:  message,
			Password: result.TemporaryPassword,
			User:     &user,
			Controls: []Control{
				{Rel: "item", Label: "View user", Method: http.MethodGet, Href: user.Href},
				{Rel: "create-form", Label: "Create another", Method: http.MethodGet, Href: "/users/new"},
			},
		})

	case errors.Is(err, service.ErrOperationPending) && result != nil:
		user := userView(result.User)
		w.Header().Set("Location", user.Href)
		wip.render(w, r, http.StatusAccepted, "result.html", resultPage{
			Title:   "Creation in progress",
			Kind:    "info",
			Message: "Keycloak did not answer in time. The username and email are reserved and the reconciler will finish the creation. The temporary password is not known: once the user is CREATED, use “Reset temporary password”.",
			User:    &user,
			Controls: []Control{
				{Rel: "item", Label: "View user", Method: http.MethodGet, Href: user.Href},
				{Rel: "collection", Label: "Back to users", Method: http.MethodGet, Href: "/users"},
			},
		})

	case errors.Is(err, service.ErrIdentifierTaken):
		form.Error = "This username or email is already registered, in this realm or in another one."
		wip.render(w, r, http.StatusConflict, "new_user.html", form)

	case errors.Is(err, service.ErrUnknownRealm):
		form.Error = "Choose one of the listed realms."
		wip.render(w, r, http.StatusUnprocessableEntity, "new_user.html", form)

	default:
		slog.ErrorContext(r.Context(), "user creation failed", "error", err)
		form.Error = "Keycloak rejected the creation: " + err.Error()
		wip.render(w, r, http.StatusBadGateway, "new_user.html", form)
	}
}

func validateNewUser(form newUserPage) string {
	switch {
	case form.Username == "" || form.Email == "" || form.Realm == "":
		return "Username, email and realm are required."
	case len(form.Username) > 100 || len(form.Email) > 100:
		return "Username and email must be at most 100 characters."
	case strings.ContainsAny(form.Username, " \t"):
		return "The username cannot contain spaces."
	case strings.Count(form.Email, "@") != 1 || strings.HasPrefix(form.Email, "@") || strings.HasSuffix(form.Email, "@"):
		return "Enter a valid email address."
	}
	return ""
}

// ---- single user -----------------------------------------------------------

type userPage struct {
	Title string
	User  UserView
}

func (wip *WebInboundPort) showUser(w http.ResponseWriter, r *http.Request) {
	user, ok := wip.loadUser(w, r)
	if !ok {
		return
	}
	wip.render(w, r, http.StatusOK, "user.html", userPage{Title: user.Username, User: userView(user)})
}

type confirmDeletePage struct {
	Title  string
	User   UserView
	Action string
}

func (wip *WebInboundPort) confirmDelete(w http.ResponseWriter, r *http.Request) {
	user, ok := wip.loadUser(w, r)
	if !ok {
		return
	}
	view := userView(user)
	if !hasRel(view.Controls, "delete") {
		wip.renderError(w, r, http.StatusConflict, "This user cannot be deleted while it is "+view.State+".")
		return
	}
	wip.render(w, r, http.StatusOK, "confirm_delete.html", confirmDeletePage{
		Title:  "Delete " + user.Username,
		User:   view,
		Action: view.Href + "/delete",
	})
}

func (wip *WebInboundPort) deleteUser(w http.ResponseWriter, r *http.Request) {
	user, ok := wip.loadUser(w, r)
	if !ok {
		return
	}

	err := wip.ControlPlaneService.DeleteUser(r.Context(), user.ID)
	switch {
	case err == nil:
		http.Redirect(w, r, "/users?notice=deleted", http.StatusSeeOther)
	case errors.Is(err, service.ErrOperationPending):
		http.Redirect(w, r, "/users?notice=deletion-pending", http.StatusSeeOther)
	case errors.Is(err, service.ErrInvalidState):
		wip.renderError(w, r, http.StatusConflict, "This user cannot be deleted in its current state.")
	case errors.Is(err, service.ErrNotFound):
		wip.renderError(w, r, http.StatusNotFound, "No user with this id. It may have been deleted.")
	default:
		slog.ErrorContext(r.Context(), "user deletion failed", "user_id", user.ID, "error", err)
		wip.renderError(w, r, http.StatusBadGateway, "Keycloak refused the deletion: "+err.Error())
	}
}

func (wip *WebInboundPort) resetPassword(w http.ResponseWriter, r *http.Request) {
	user, ok := wip.loadUser(w, r)
	if !ok {
		return
	}
	view := userView(user)
	if !hasRel(view.Controls, "reset-password") {
		wip.renderError(w, r, http.StatusConflict, "The password can only be reset for a CREATED user.")
		return
	}

	password, err := wip.ControlPlaneService.ResetTemporaryPassword(r.Context(), user.ID)
	if err != nil {
		slog.ErrorContext(r.Context(), "password reset failed", "user_id", user.ID, "error", err)
		wip.renderError(w, r, http.StatusBadGateway, "Could not reset the password: "+err.Error())
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	wip.render(w, r, http.StatusOK, "result.html", resultPage{
		Title:    "Temporary password reset",
		Kind:     "success",
		Message:  "The previous password no longer works. Give the user this one: Keycloak asks for a new one at the first login.",
		Password: password,
		User:     &view,
		Controls: []Control{
			{Rel: "item", Label: "View user", Method: http.MethodGet, Href: view.Href},
			{Rel: "collection", Label: "Back to users", Method: http.MethodGet, Href: "/users"},
		},
	})
}

func hasRel(controls []Control, rel string) bool {
	for _, c := range controls {
		if c.Rel == rel {
			return true
		}
	}
	return false
}
