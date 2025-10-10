package main

import (
	"fmt"
	"net/http"
	"sync/atomic"

	"github.com/jonathangibson/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	Queries        *database.Queries // go
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) writeNumberOfRequests(w http.ResponseWriter, r *http.Request) {
	hits := cfg.fileserverHits.Load()
	response := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", hits)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte(response))
}

func (cfg *apiConfig) resetHitsHandler(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("Hits reset"))
}
