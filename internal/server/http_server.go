package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type HTTPServer struct {
	Handler http.Handler
}

func NewHTTPServer() *HTTPServer {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return &HTTPServer{Handler: r}
}
