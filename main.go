package main

import (
	"database/sql"
	"encoding/json"
	"log"      // logging errors and info
	"net/http" // web server and HTTP utilities
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/jonathangibson/chirpy/internal/database"
	_ "github.com/lib/pq"
)

type errorResponse struct {
	Error string `json:"error"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserId    uuid.UUID `json:"user_id"`
}

var naughty = map[string]struct{}{
	"kerfuffle": {},
	"sharbert":  {},
	"fornax":    {},
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8") // set response header
	w.WriteHeader(200)                                          // set HTTP status code
	w.Write([]byte("OK"))                                       // write response body
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) error {
	dat, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
	return nil
}

func stripProfane(s string) string {
	words := strings.Split(s, " ")
	for i, w := range words {
		if _, ok := naughty[strings.ToLower(w)]; ok {
			words[i] = "****"
		}
	}
	return strings.Join(words, " ")
}

//replaced w/ cfg.chirpsHandler
/*
func validateHandler(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		respondWithJSON(w, 400, errorResponse{Error: err.Error()})
		return
	}
	// params is a struct with data populated successfully

	type errorVals struct {
		Error string `json:"error"`
	}

	type validVals struct {
		CleanedBody string `json:"cleaned_body"`
	}

	if len(params.Body) > 140 {
		err := respondWithJSON(w, 400, errorVals{Error: "Chirp is too long"})
		if err != nil {
			log.Printf("Error responding with JSON: %s", err)
		}
		return
	}
	respondWithJSON(w, 200, validVals{CleanedBody: stripProfane(params.Body)})
}
*/

func routes(cfg *apiConfig) http.Handler {
	mux := http.NewServeMux()                          // create a new router
	mux.HandleFunc("GET /api/healthz", healthzHandler) // register handler for /healthz
	mux.HandleFunc("GET /admin/metrics", cfg.writeNumberOfRequests)
	//mux.HandleFunc("POST /admin/reset", cfg.resetHitsHandler)
	//mux.HandleFunc("POST /api/validate_chirp", validateHandler)
	mux.HandleFunc("POST /api/users", cfg.addUserHandler)
	mux.HandleFunc("POST /admin/reset", cfg.resetHandler)
	mux.HandleFunc("POST /api/chirps", cfg.chirpsHandler)
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("."))))) // register file server for /app/
	mux.Handle("/app", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))  // and for just /app because why not
	return mux                                                                                              // return the router
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("warning: .env not loaded:", err)
	}
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL is empty")
	}
	platform := os.Getenv("PLATFORM")
	if platform == "" {
		platform = "prod"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	dbQueries := database.New(db)

	cfg := apiConfig{
		Queries:  dbQueries,
		Platform: platform,
	}

	log.Println("Now starting server...!")
	log.Fatal(http.ListenAndServe(":8080", routes(&cfg)))
}
