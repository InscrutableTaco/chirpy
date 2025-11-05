package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
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
	ApiKey         string
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

	// struct for decoding into
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// decode the request body
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithJSON(w, 400, errorResponse{Error: err.Error()})
		return
	}

	// format email/password
	email := strings.TrimSpace(params.Email)
	pwd := strings.TrimSpace(params.Password)
	if email == "" || pwd == "" {
		respondWithJSON(w, 400, errorResponse{Error: "email and password are required"})
		return
	}

	// hash password
	hashPass, err := auth.HashPassword(pwd)
	if err != nil {
		log.Printf("Error hashing password: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"})
		return
	}

	// params for adding user row
	dbParams := database.CreateUserParams{
		Email:          params.Email,
		HashedPassword: hashPass,
	}

	// add a user row
	user, err := cfg.Queries.CreateUser(r.Context(), dbParams)
	if err != nil {
		log.Printf("Error creating user: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"})
		return
	}

	// struct for response body
	responseUser := User{
		ID:         user.ID,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
		Email:      user.Email,
		IsUpgraded: user.IsChirpyRed,
	}

	// success msg
	respondWithJSON(w, 201, responseUser)

}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {

	// verify environment is dev
	if cfg.Platform != "dev" {
		respondWithJSON(w, 403, errorResponse{Error: "403 Forbidden"})
		return
	}

	// delete all users from the database
	err := cfg.Queries.DeleteUsers(r.Context())
	if err != nil {
		log.Printf("Error deleting users: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
		return
	}

	// delete all chirps from the database
	err = cfg.Queries.DeleteChirps(r.Context())
	if err != nil {
		log.Printf("Error deleting chirps: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
		return
	}

	//reset page hits counter
	cfg.fileserverHits.Store(0)

	// send success message
	respondWithJSON(w, 200, map[string]string{"status": "ok"})
}

func (cfg *apiConfig) loginHandler(w http.ResponseWriter, r *http.Request) {

	// declare struct for login params
	type parameters struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// decode params from request
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithJSON(w, 400, errorResponse{Error: err.Error()})
		return
	}

	// lookup user by email
	email := strings.TrimSpace(params.Email)
	user, err := cfg.Queries.GetUserByEmail(r.Context(), email)
	if err != nil {
		log.Printf("Error locating user: %s", err)
		respondWithJSON(w, 401, errorResponse{Error: "Incorrect email or password"})
		return
	}

	// compare password hashes
	pwd := strings.TrimSpace(params.Password)
	match, err := auth.CheckPasswordHash(pwd, user.HashedPassword)
	if !match || err != nil {
		log.Printf("Password mismatch or error")
		respondWithJSON(w, 401, errorResponse{Error: "Incorrect email or password"})
		return
	}

	// create user for response
	responseUser := User{
		ID:         user.ID,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
		Email:      user.Email,
		IsUpgraded: user.IsChirpyRed,
	}

	// create a json web token
	tok, err := auth.MakeJWT(user.ID, cfg.Secret, time.Duration(time.Hour))
	if err != nil {
		log.Printf("error: %s", err)
		respondWithJSON(w, 500, responseUser)
		return
	}

	// create a refresh token
	refreshTok, err := auth.MakeRefreshToken()
	if err != nil {
		log.Printf("error: %s", err)
		respondWithJSON(w, 500, responseUser)
		return
	}

	// store refresh token in the database
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
		return
	}

	// add both tokens to the user struct for response
	responseUser.Token = tok
	responseUser.RefreshToken = refreshTok

	// send successful response
	respondWithJSON(w, 200, responseUser)

}

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {

	// authenticate the user
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("bearer token not found")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// struct for decoding body
	type createChirpDTO struct {
		Body string `json:"body"`
	}
	var dto createChirpDTO

	// decode the body into the struct
	err = json.NewDecoder(r.Body).Decode(&dto)
	if err != nil {
		respondWithJSON(w, 400, errorResponse{Error: "invalid JSON"})
		return
	}

	// validate chirp length
	if len(dto.Body) > 140 {
		respondWithJSON(w, 400, errorResponse{Error: "Chirp is too long"})
		return
	}

	// check for authorization
	tokenId, err := auth.ValidateJWT(tok, cfg.Secret)
	if err != nil {
		log.Printf("Error validating token: %s", err.Error())
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// params for adding chirp
	params := database.CreateChirpParams{
		Body:   stripProfane(dto.Body),
		UserID: tokenId,
	}

	// add the chirp
	chirp, err := cfg.Queries.CreateChirp(r.Context(), params)
	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Error creating chirp"})
		return
	}

	// success response
	respondWithJSON(w, 201, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	})

}

func (cfg *apiConfig) getChirpsHandler(w http.ResponseWriter, r *http.Request) {

	// id, sort criteria from url
	idStr := r.URL.Query().Get("author_id")
	sortStr := r.URL.Query().Get("sort")

	var chirps []database.Chirp
	var err error

	// id specified?
	if idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			log.Printf("Error parsing id string: %s", err.Error())
			respondWithJSON(w, 401, "Invalid user id")
			return
		}

		// get user's chirps if so
		chirps, err = cfg.Queries.GetUserChirps(r.Context(), id)
		if err != nil {
			respondWithJSON(w, 500, errorResponse{Error: err.Error()})
			return
		}
	} else { // otherwise get all chirps
		chirps, err = cfg.Queries.GetAllChirps(r.Context())
		if err != nil {
			respondWithJSON(w, 500, errorResponse{Error: err.Error()})
			return
		}
	}

	chirpSlice := make([]Chirp, 0, len(chirps))

	// prepare struct for response
	for _, c := range chirps {
		chirpSlice = append(chirpSlice, Chirp{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserId:    c.UserID,
		})
	}

	// sorting
	if sortStr == "desc" {
		sort.Slice(chirpSlice, func(i, j int) bool { return chirpSlice[i].CreatedAt.After(chirpSlice[j].CreatedAt) })
	}

	// response body with chirps
	respondWithJSON(w, 200, chirpSlice)

}

func (cfg *apiConfig) getOneChirpHandler(w http.ResponseWriter, r *http.Request) {

	// get chirp id from request
	idStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(idStr)
	if err != nil {
		respondWithJSON(w, 400, errorResponse{Error: "Unable to parse chirp id"})
		return
	}

	// retrieve chirp from database
	chirp, err := cfg.Queries.GetOneChirp(r.Context(), chirpID)
	if errors.Is(err, sql.ErrNoRows) {
		respondWithJSON(w, 404, errorResponse{Error: "not found"})
		return
	}
	if err != nil {
		respondWithJSON(w, 500, errorResponse{Error: "Internal error"})
		return
	}

	// return json body with chirp struct
	respondWithJSON(w, 200, Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	})

}

func (cfg *apiConfig) refreshHandler(w http.ResponseWriter, r *http.Request) {

	// obtain refresh token from request
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("bearer token not found")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// retrieve user id from refresh token in database
	user, err := cfg.Queries.GetUserFromRefreshToken(r.Context(), tok)
	if err != nil {
		log.Printf("refresh token lookup failed")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// create a new json web token
	newTok, err := auth.MakeJWT(user.ID, cfg.Secret, time.Duration(time.Hour))
	if err != nil {
		log.Printf("error: %s", err)
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// send success msg
	respondWithJSON(w, 200, map[string]string{"token": newTok})
}

func (cfg *apiConfig) revokeHandler(w http.ResponseWriter, r *http.Request) {

	// get token
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("bearer token not found")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// revoke refresh token
	err = cfg.Queries.RevokeRefreshToken(r.Context(), tok)
	if err != nil {
		log.Printf("could not retrieve refresh token")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// success header
	w.WriteHeader(204)

}

func (cfg *apiConfig) updateUserHandler(w http.ResponseWriter, r *http.Request) {

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

	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("bearer token not found")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	userId, err := auth.ValidateJWT(tok, cfg.Secret)
	if err != nil {
		log.Printf("Error validating token: %s", err.Error())
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
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
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"})
		return
	}

	dbParams := database.UpdateUserParams{
		Email:          email,
		HashedPassword: hashPass,
		ID:             userId,
	}

	err = cfg.Queries.UpdateUser(r.Context(), dbParams)
	if err != nil {
		log.Printf("Error updating user: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"})
		return
	}

	user, err := cfg.Queries.GetUserByID(r.Context(), userId)
	if err != nil {
		log.Printf("Error retrieving user after updating: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"})
		return
	}

	responseUser := User{
		ID:         user.ID,
		CreatedAt:  user.CreatedAt,
		UpdatedAt:  user.UpdatedAt,
		Email:      user.Email,
		IsUpgraded: user.IsChirpyRed,
	}

	respondWithJSON(w, 200, responseUser)

}

func (cfg *apiConfig) deleteChirpHandler(w http.ResponseWriter, r *http.Request) {

	// parse chirp id
	idStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(idStr)
	if err != nil {
		respondWithJSON(w, 400, errorResponse{Error: "Unable to parse chirp id"})
		return
	}

	// authenticate user
	tok, err := auth.GetBearerToken(r.Header)
	if err != nil {
		log.Printf("bearer token not found")
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}
	userId, err := auth.ValidateJWT(tok, cfg.Secret)
	if err != nil {
		log.Printf("Error validating token: %s", err.Error())
		respondWithJSON(w, 401, errorResponse{Error: "Unauthorized"})
		return
	}

	// fetch chirp
	chirp, err := cfg.Queries.GetOneChirp(r.Context(), chirpID)
	if errors.Is(err, sql.ErrNoRows) {
		respondWithJSON(w, 404, errorResponse{Error: "not found"})
		return
	}
	if err != nil {
		respondWithJSON(w, 500, errorResponse{Error: "Internal error"})
		return
	}

	// check author
	if chirp.UserID != userId {
		log.Printf("User id %s unauthorized to delete chirp id %s", userId, chirpID)
		respondWithJSON(w, 403, errorResponse{Error: "Forbidden"})
		return
	}

	// delete chirps from the database
	err = cfg.Queries.DeleteChirp(r.Context(), chirp.ID)
	if err != nil {
		log.Printf("Error deleting chirp: %s", err.Error())
		respondWithJSON(w, 500, errorResponse{Error: "Internal server error"})
		return
	}

	// set success header
	w.WriteHeader(204)

}

func (cfg *apiConfig) upgradeHandler(w http.ResponseWriter, r *http.Request) {

	// get api key from request
	key, err := auth.GetAPIKey(r.Header)
	if err != nil {
		log.Printf("api key not found")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// authenticate key
	if key != cfg.ApiKey {
		log.Printf("api key doesn't match")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// struct to receive request params
	type parameters struct {
		Event string `json:"event"`
		Data  struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}

	// decode the request body
	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err = decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// not an upgrade request? return
	if params.Event != "user.upgraded" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// parse the user id
	id, err := uuid.Parse(params.Data.UserID)
	if err != nil {
		log.Printf("Error parsing user id: %s", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// upgrade the user (upgrades ALL rows matching the id)
	rows, err := cfg.Queries.UpgradeUser(r.Context(), id)

	// error handling the query
	if err != nil {
		log.Printf("Error upgrading user: %s", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if rows == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// success
	w.WriteHeader(http.StatusNoContent)

}
