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
	"time"

	"github.com/google/uuid"
	"github.com/jonathangibson/chirpy/internal/auth"
	"github.com/jonathangibson/chirpy/internal/database"
)

type apiConfig struct {
	fileserverHits atomic.Int32
	Queries        *database.Queries // go
	Platform       string
	Secret         string
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

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {

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

	user, err := cfg.Queries.GetUserByEmail(r.Context(), email)
	if err != nil {
		log.Printf("Error locating user: %s", err)
		respondWithJSON(w, 401, errorResponse{Error: "Incorrect email or password"})
		return
	}

	match, err := auth.CheckPasswordHash(pwd, user.HashedPassword)
	if !match || err != nil {
		log.Printf("Password mismatch or error")
		respondWithJSON(w, 401, errorResponse{Error: "Incorrect email or password"})
		return
	}

	responseUser := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Email:     user.Email,
	}

	tok, err := auth.MakeJWT(user.ID, cfg.Secret, time.Duration(time.Hour))
	if err != nil {
		log.Printf("error: %s", err)
		respondWithJSON(w, 500, responseUser)
	}

	refreshTok, err := auth.MakeRefreshToken()
	if err != nil {
		log.Printf("error: %s", err)
		respondWithJSON(w, 500, responseUser)
	}

	_, err = cfg.Queries.CreateRefreshToken(r.Context(), database.CreateRefreshTokenParams{
		Token:  refreshTok,
		UserID: user.ID,
		ExpiresAt: sql.NullTime{
			Time:  time.Now().Add(60 * 24 * time.Hour),
			Valid: true,
		},
	})
	if err != nil {
		log.Printf("error: %s", err)
		respondWithJSON(w, 500, responseUser)
	}

	responseUser.Token = tok
	responseUser.RefreshToken = refreshTok

	respondWithJSON(w, 200, responseUser)

}

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {

	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("Could not get token")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	type createChirpDTO struct {
		Body string `json:"body"`
	}

	var dto createChirpDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		respondWithJSON(w, 400, errorResponse{Error: "invalid JSON"})
		return
	}

	if len(dto.Body) > 140 {
		respondWithJSON(w, 400, errorResponse{Error: "Chirp is too long"})
		return
	}

	tokenId, err := auth.ValidateJWT(tok, cfg.Secret)
	if err != nil {
		log.Printf("Error validating token: %s", err.Error())
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	params := database.CreateChirpParams{
		Body:   stripProfane(dto.Body),
		UserID: tokenId,
	}

	chirp, err := cfg.Queries.CreateChirp(r.Context(), params)
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Error creating chirp"})
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
