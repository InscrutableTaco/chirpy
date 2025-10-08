package main

import (
	"encoding/json"
	"log"      // logging errors and info
	"net/http" // web server and HTTP utilities
)

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

func validateHandler(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}
	// params is a struct with data populated successfully

	type errorVals struct {
		Error string `json:"error"`
	}

	type validVals struct {
		Valid bool `json:"valid"`
	}

	if len(params.Body) > 140 {
		err := respondWithJSON(w, 400, errorVals{Error: "Chirp is too long"})
		if err != nil {
			log.Printf("Error responding with JSON: %s", err)
		}
		return
	}
	respondWithJSON(w, 200, validVals{Valid: true})
}

func routes(cfg *apiConfig) http.Handler {
	mux := http.NewServeMux()                          // create a new router
	mux.HandleFunc("GET /api/healthz", healthzHandler) // register handler for /healthz
	mux.HandleFunc("GET /admin/metrics", cfg.writeNumberOfRequests)
	mux.HandleFunc("POST /admin/reset", cfg.resetHitsHandler)
	mux.HandleFunc("POST /api/validate_chirp", validateHandler)
	mux.Handle("/app/", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir("."))))) // register file server for /app/
	mux.Handle("/app", cfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))  // and for just /app because why not
	return mux                                                                                              // return the router
}

func main() {
	cfg := apiConfig{}
	log.Println("Now starting server...!")                // log startup message
	log.Fatal(http.ListenAndServe(":8080", routes(&cfg))) // start server on port 8080 with configured routes
}
