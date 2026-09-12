package user

import (
	"errors"
	"net/http"

	"github.com/thvnhtai/gearshare/internal/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || len(req.Password) < 8 || req.DisplayName == "" {
		httputil.Error(w, http.StatusBadRequest, "email, display_name and a password of at least 8 characters are required")
		return
	}

	role := Role(req.Role)
	if role != RoleOwner {
		role = RoleRenter
	}

	result, err := h.service.Register(r.Context(), RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		Role:        role,
	})
	if errors.Is(err, ErrEmailTaken) {
		httputil.Error(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not register user")
		return
	}

	httputil.JSON(w, http.StatusCreated, toAuthResponse(result))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		httputil.Error(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not log in")
		return
	}

	httputil.JSON(w, http.StatusOK, toAuthResponse(result))
}
