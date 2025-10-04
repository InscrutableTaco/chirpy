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

func validateHandler(w http.ResponseWriter, r *http.Request) {

	type parameters struct {
		// these tags indicate how the keys in the JSON should be mapped to the struct fields
		// the struct fields must be exported (start with a capital letter) if you want them parsed
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		// an error will be thrown if the JSON is invalid or has the wrong types
		// any missing fields will simply have their values in the struct set to their zero value
		log.Printf("Error decoding parameters: %s", err)
		w.WriteHeader(500)
		return
	}
	// params is a struct with data populated successfully
	// ...

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
