package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
)

type sumResponse struct {
	A   int `json:"a"`
	B   int `json:"b"`
	Sum int `json:"sum"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/sum", sumHandler)
	mux.HandleFunc("/healthz", healthHandler)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: mux,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}

func sumHandler(w http.ResponseWriter, r *http.Request) {
	a, err := parseIntQuery(r, "a")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	b, err := parseIntQuery(r, "b")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sumResponse{
		A:   a,
		B:   b,
		Sum: a + b,
	})
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func parseIntQuery(r *http.Request, key string) (int, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return 0, fmt.Errorf("missing query parameter %q", key)
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid integer value for %q: %s", key, value)
	}
	return number, nil
}
