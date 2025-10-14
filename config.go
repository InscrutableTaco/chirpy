package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"

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
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)

	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithJSON(w, 400, errorResponse{Error: err.Error()})
		return
	}

	user, err := cfg.Queries.CreateUser(r.Context(), params.Email)

	if err != nil {
		log.Printf("Error creating user: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
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
	cfg.fileserverHits.Store(0)
	respondWithJSON(w, 200, map[string]string{"status": "ok"})
}

func (cfg *apiConfig) chirpsHandler(w http.ResponseWriter, r *http.Request) {

	/*
		type parameters struct {
			Body   string `json:"body"`
			UserId string `json:"user_id"`
		}
	*/

	decoder := json.NewDecoder(r.Body)
	params := database.CreateChirpParams{} //parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithJSON(w, 400, errorResponse{Error: err.Error()})
		return
	}

	type errorVals struct {
		Error string `json:"error"`
	}

	/*
		type validVals struct {
			CleanedBody string `json:"cleaned_body"`
		}
	*/

	if len(params.Body) > 140 {
		err := respondWithJSON(w, 400, errorVals{Error: "Chirp is too long"})
		if err != nil {
			log.Printf("Error responding with JSON: %s", err)
		}
		return
	}

	//respondWithJSON(w, 200, validVals{CleanedBody: stripProfane(params.Body)})

	params.Body = stripProfane(params.Body)

	chirp, err := cfg.Queries.CreateChirp(r.Context(), params)

	if err != nil {
		log.Printf("Error creating chirp: %s", err)
		respondWithJSON(w, 500, errorResponse{Error: err.Error()})
		return
	}

	responseChirp := Chirp{
		ID:        chirp.ID,
		CreatedAt: chirp.CreatedAt,
		UpdatedAt: chirp.UpdatedAt,
		Body:      chirp.Body,
		UserId:    chirp.UserID,
	}

	respondWithJSON(w, 201, responseChirp)

}
