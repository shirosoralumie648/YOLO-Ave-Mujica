package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"yolo-ave-mujica/internal/datahub"
)

type HTTPServer struct {
	Handler http.Handler
}

func NewHTTPServer() *HTTPServer {
	return NewHTTPServerWithDataHub(nil)
}

func NewHTTPServerWithDataHub(dataHubHandler *datahub.Handler) *HTTPServer {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	if dataHubHandler != nil {
		r.Route("/v1", func(r chi.Router) {
			r.Post("/datasets", dataHubHandler.CreateDataset)
			r.Post("/datasets/{id}/snapshots", dataHubHandler.CreateSnapshot)
			r.Get("/datasets/{id}/snapshots", dataHubHandler.ListSnapshots)
			r.Post("/objects/presign", dataHubHandler.PresignObject)
		})
	}

	return &HTTPServer{Handler: r}
}
