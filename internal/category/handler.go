package category

import (
	"net/http"

	"github.com/thvnhtai/gearshare/internal/httputil"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	cats, err := h.repo.List(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "could not load categories")
		return
	}
	httputil.JSON(w, http.StatusOK, cats)
}
