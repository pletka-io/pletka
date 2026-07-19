package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/pletka-io/pletka/pkg/domain"
	"github.com/pletka-io/pletka/pkg/ids"
	"github.com/pletka-io/pletka/pkg/session"
	"github.com/jackc/pgx/v5/pgconn"
)

// AuthHandler handles authentication-related HTTP requests.
// Login, Register, Me, and Logout read+write through domain.WeaveStore
// (Auth() + Memberships()) and the server-side session manager.
type AuthHandler struct {
	logger     *slog.Logger
	weave      domain.WeaveStore
	sessionMgr *session.Manager
}

// NewAuthHandler creates a new authentication handler.
func NewAuthHandler(logger *slog.Logger, weave domain.WeaveStore, sm *session.Manager) *AuthHandler {
	if logger == nil {
		panic("logger is nil in NewAuthHandler")
	}
	return &AuthHandler{
		logger:     logger,
		weave:      weave,
		sessionMgr: sm,
	}
}

// NewAuthHandlerForTests builds a minimal AuthHandler for integration
// tests. Same shape as the production constructor; the dedicated
// helper stays for test ergonomics.
func NewAuthHandlerForTests(weave domain.WeaveStore, sm *session.Manager) *AuthHandler {
	return &AuthHandler{
		weave:      weave,
		sessionMgr: sm,
		logger:     slog.Default(),
	}
}

// dummyBcryptHash is a pre-computed bcrypt hash of a random string.
// Used to make the no-such-user login path run the same bcrypt.CompareHashAndPassword
// work as the found-user path, so response time does not reveal whether the
// login identifier exists.
//
// GENERATED ONCE. Safe to commit: this hash is not tied to any real password
// and will never validate against any user-supplied input.
const dummyBcryptHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5CpPqUzqWH7X6xhY5vCxKzvMVsL5K"

// RegisterRequest structure for user registration
type RegisterRequest struct {
	Name            string `json:"name" validate:"required,min=2"`
	Email           string `json:"email" validate:"required,email"`
	Username        string `json:"username" validate:"required,min=3,alphanum"`
	Password        string `json:"password" validate:"required,min=8"`
	ConfirmPassword string `json:"confirm_password" validate:"required,eqfield=Password"`
	ORCID           string `json:"orcid,omitempty" validate:"omitempty"`
	Institution     string `json:"institution,omitempty" validate:"omitempty"`
}

// AuthResponse is the legacy response envelope used by Login,
// Register, and the error helpers below. Token / User fields removed
// alongside the GORM purge. Login no longer issues a JWT (the
// session middleware owns the auth state) and User went away with
// userToResponse.
type AuthResponse struct {
	Success bool              `json:"success"`
	Message string            `json:"message,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
}

// Login authenticates a user by email or slug and starts a session.
// Request: {login, password} where login is email or slug.
// Response 200: {actor_id, email, display_name, slug}.
// Response 401 on invalid credentials; 500 on internal error.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		// Login accepts either an email address or a slug (username).
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Login == "" || req.Password == "" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	ctx := r.Context()
	rec, err := h.weave.Auth().GetByEmailOrSlug(ctx, req.Login)
	if err != nil {
		slog.Error("lookup for login", "err", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if rec == nil {
		// Constant-time: still do a dummy bcrypt to avoid a timing oracle.
		_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(req.Password))
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(rec.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := h.weave.Auth().MarkLogin(ctx, rec.ActorID); err != nil {
		slog.Warn("mark login", "err", err) // non-fatal
	}

	if err := h.sessionMgr.EstablishAuthenticatedSession(ctx, rec.ActorID, rec.Email); err != nil {
		h.logger.Error("establish session", "err", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"actor_id":     rec.ActorID,
		"email":        rec.Email,
		"display_name": rec.DisplayName,
		"slug":         rec.Slug,
	})
}

// Register creates a new person actor and auth row, then starts a session.
// Request: {name, email, username, password}.
// Response 201: {actor_id, email, username, display_name}.
// Response 409 on duplicate email/slug; 422 on validation errors.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	errs := map[string][]string{}
	if strings.TrimSpace(req.Name) == "" {
		errs["name"] = append(errs["name"], "required")
	}
	if strings.TrimSpace(req.Email) == "" {
		errs["email"] = append(errs["email"], "required")
	}
	if strings.TrimSpace(req.Username) == "" {
		errs["username"] = append(errs["username"], "required")
	} else if err := ValidateSlug(req.Username); err != nil {
		errs["username"] = append(errs["username"], err.Error())
	}
	if len(req.Password) < 12 {
		errs["password"] = append(errs["password"], "password must be at least 12 characters")
	}
	if len(errs) > 0 {
		writeValidationErrors(w, errs)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("bcrypt", "err", err)
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	ctx := r.Context()
	actorID := ids.GenerateULID()

	profile, err := h.weave.Auth().RegisterPerson(ctx, domain.RegisterPersonParams{
		ActorID:      actorID,
		DisplayName:  strings.TrimSpace(req.Name),
		Slug:         strings.ToLower(strings.TrimSpace(req.Username)),
		Email:        strings.ToLower(strings.TrimSpace(req.Email)),
		PasswordHash: string(hash),
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "email or username already in use")
			return
		}
		slog.Error("register", "err", err)
		writeError(w, http.StatusInternalServerError, "could not register user")
		return
	}

	h.sessionMgr.Put(ctx, session.KeyUserID, profile.ActorID)
	h.sessionMgr.Put(ctx, session.KeyUserEmail, profile.Email)
	h.sessionMgr.Put(ctx, session.KeyIsAuthenticated, true)

	writeJSON(w, http.StatusCreated, map[string]any{
		"actor_id":     profile.ActorID,
		"email":        profile.Email,
		"username":     profile.Slug,
		"display_name": profile.DisplayName,
	})
}

// Logout destroys the current session and returns 204 No Content.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_ = h.sessionMgr.Destroy(ctx)
	w.WriteHeader(http.StatusNoContent)
}

// Me returns the current user's profile. 401 when anonymous. 500 on DB error.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	snap := FromContext(ctx)
	if snap.IsAnonymous {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	profile, err := h.weave.Auth().GetProfileByActorID(ctx, snap.ActorID)
	if err != nil {
		slog.Error("me lookup", "err", err, "actor_id", snap.ActorID)
		writeError(w, http.StatusInternalServerError, "could not load profile")
		return
	}
	if profile == nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"actor_id":     profile.ActorID,
		"slug":         profile.Slug,
		"display_name": profile.DisplayName,
		"email":        profile.Email,
	})
}

// GetProfile and UpdateProfile are gone:
// - GetProfile is replaced by Me above (weave-native, no GORM Actors).
// - UpdateProfile is replaced by PUT /me, served by pkg/weave/actoradmin
//   (schema-driven via FormRenderer).

func (h *AuthHandler) validateRegisterRequest(req RegisterRequest) map[string]string {
	errors := make(map[string]string)

	if strings.TrimSpace(req.Name) == "" {
		errors["name"] = "Name is required"
	} else if len(req.Name) < 2 {
		errors["name"] = "Name must be at least 2 characters"
	}

	if strings.TrimSpace(req.Email) == "" {
		errors["email"] = "Email is required"
	} else if !strings.Contains(req.Email, "@") {
		errors["email"] = "Please enter a valid email address"
	}

	if strings.TrimSpace(req.Username) == "" {
		errors["username"] = "Username is required"
	} else if len(req.Username) < 3 {
		errors["username"] = "Username must be at least 3 characters"
	}

	if req.Password == "" {
		errors["password"] = "Password is required"
	} else if len(req.Password) < 8 {
		errors["password"] = "Password must be at least 8 characters"
	}

	if req.ConfirmPassword == "" {
		errors["confirm_password"] = "Password confirmation is required"
	} else if req.Password != req.ConfirmPassword {
		errors["confirm_password"] = "Passwords do not match"
	}

	return errors
}

func (h *AuthHandler) sendJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON response", "error", err, "status", status)
	}
}

func (h *AuthHandler) sendErrorResponse(w http.ResponseWriter, status int, message string, errors map[string]string) {
	response := AuthResponse{
		Success: false,
		Message: message,
		Errors:  errors,
	}
	h.sendJSONResponse(w, status, response)
}

// writeJSON writes v as a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a simple error response: {"error": msg}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeValidationErrors writes a 422 response shaped for FormRenderer:
// {"errors": {"field_name": ["msg1", "msg2"]}}.
func writeValidationErrors(w http.ResponseWriter, fields map[string][]string) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
		"errors": fields,
	})
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (sqlstate 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
