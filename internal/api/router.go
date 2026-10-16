package api

import (
	"io"
	"net/http"
)

const healthyPayload = `{"status":"ok"}` + "\n"

func NewRouter() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	return mux
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	if _, err := io.WriteString(w, healthyPayload); err != nil {
		return
	}

}
