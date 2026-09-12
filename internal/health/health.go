package health

import (
	"net/http"

	"github.com/thvnhtai/gearshare/internal/httputil"
)

type Response struct {
	Status string `json:"status"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	httputil.JSON(w, http.StatusOK, Response{Status: "ok"})
}
