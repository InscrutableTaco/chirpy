package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jonathangibson/chirpy/internal/auth"
	"github.com/jonathangibson/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	Queries        *database.Queries // go
	Platform       string
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

/*
func (cfg *apiConfig) resetHitsHandler(w http.ResponseWriter, r *http.Request) {
	cfg.fileserverHits.Store(0) // only need this part added to the new func
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("Hits reset"))
}
*/

func (cfg *apiConfig) addUserHandler(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithJSON(w, 400, errorResponse{Error: err.Error()})
		return
	}

	email := strings.TrimSpace(params.Email)
	pwd := strings.TrimSpace(params.Password)
	if email == "" || pwd == "" {
		respondWithJSON(w, 400, errorResponse{Error: "email and password are required"})
		return
	}

	hashPass, err := auth.HashPassword(pwd)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"}) // Is 400 correct?
		return
	}

	dbParams := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashPass,
	}

	user, err := cfg.Queries.CreateUser(r.Context(), dbParams)

	if err != nil {
		log.Printf("Error creating user: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"})
		return
	}

	responseUser := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}

	respondWithJSON(w, 201, responseUser)

}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {

	if cfg.Platform != "dev" {
		respondWithJSON(w, 403, errorResponse{Error: "403 Forbidden"})
		return
	}

	err := cfg.Queries.DeleteUsers(r.Context())
	if err != nil {
		log.Printf("Error deleting users: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
		return
	}

	err = cfg.Queries.DeleteChirps(r.Context())
	if err != nil {
		log.Printf("Error deleting chirps: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
		return
	}

	cfg.fileserverHits.Store(0)
	respondWithJSON(w, 200, map[string]string{"status": "ok"})
}

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {

	type createChirpDTO struct {
		Body   string `json:"body"`
		UserID string `json:"user_id"`
	}

	var dto createChirpDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		respondWithJSON(w, 400, errorResponse{Error: "invalid JSON"})
		return
	}

	if len(dto.Body) > 140 {
		respondWithJSON(w, 400, map[string]string{"error": "Chirp is too long"})
		return
	}

	uid, err := uuid.Parse(dto.UserID)
	if err != nil {
		respondWithJSON(w, 400, map[string]string{"error": "invalid user_id"})
		return
	}

	params := database.CreateChirpParams{
		Body:   stripProfane(dto.Body),
		UserID: uid,
	}

	chirp, err := cfg.Queries.CreateChirp(r.Context(), params)
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
		return
	}

	respondWithJSON(w, 201, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	})

}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {
	chirps, err := cfg.Queries.GetAllChirps(r.Context())

	if err != nil {
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
		return
	}

	chirpSlice := make([]Chirp, 0, len(chirps))

	for _, c := range chirps {
		chirpSlice = append(chirpSlice, Chirp{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserId:    c.UserID,
		})
	}

	respondWithJSON(w, 200, chirpSlice)

}

func (c *apiConfig) getOneChirpHandler(w http.ResponseWriter, r *http.Request) {

	idStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(idStr)

	if err != nil {
		respondWithJSON(w, 400, errorResponse{Error: "Unable to parse chirp id"})
		return
	}

	chirp, err := c.Queries.GetOneChirp(r.Context(), chirpID)

	if errors.Is(err, sql.ErrNoRows) {
		respondWithJSON(w, 404, errorResponse{Error: "not found"})
		return
	}

	if err != nil {
		respondWithJSON(w, 500, errorResponse{Error: "Internal error"})
		return
	}

	respondWithJSON(w, 200, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	})

}
