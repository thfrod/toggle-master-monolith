package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type server struct {
	db *sql.DB
}

type keyResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	APIKey    string    `json:"api_key,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func waitForDatabase(db *sql.DB) error {
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		if err := db.Ping(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		log.Printf("database unavailable (attempt %d/30): %v", attempt, lastErr)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("database did not become ready: %w", lastErr)
}

func (s *server) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS api_keys (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(100) UNIQUE NOT NULL,
			key_hash CHAR(64) UNIQUE NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`)
	return err
}

func randomAPIKey() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "tm_" + hex.EncodeToString(buffer), nil
}

func hash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "error": "database unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) keys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.QueryContext(r.Context(), "SELECT id, name, created_at FROM api_keys ORDER BY name")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list API keys"})
			return
		}
		defer rows.Close()
		result := make([]keyResponse, 0)
		for rows.Next() {
			var item keyResponse
			if err := rows.Scan(&item.ID, &item.Name, &item.CreatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not read API keys"})
				return
			}
			result = append(result, item)
		}
		writeJSON(w, http.StatusOK, result)
	case http.MethodPost:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}
		plain, err := randomAPIKey()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not generate API key"})
			return
		}
		var response keyResponse
		err = s.db.QueryRowContext(r.Context(),
			"INSERT INTO api_keys (name, key_hash) VALUES ($1, $2) RETURNING id, name, created_at",
			input.Name, hash(plain),
		).Scan(&response.ID, &response.Name, &response.CreatedAt)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "an API key with this name already exists"})
			return
		}
		response.APIKey = plain
		writeJSON(w, http.StatusCreated, response)
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (s *server) verify(w http.ResponseWriter, r *http.Request) {
	authorization := r.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]bool{"valid": false})
		return
	}
	var name string
	err := s.db.QueryRowContext(r.Context(), "SELECT name FROM api_keys WHERE key_hash = $1", hash(strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer ")))).Scan(&name)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusUnauthorized, map[string]bool{"valid": false})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not verify API key"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "name": name})
}

func main() {
	dsn := env("AUTH_DATABASE_URL", "postgres://user:password@postgres-auth:5432/auth?sslmode=disable")
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	s := &server{db: db}
	if err := waitForDatabase(db); err != nil {
		log.Fatal(err)
	}
	if err := s.initSchema(); err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/auth/keys", s.keys)
	mux.HandleFunc("/auth/verify", s.verify)
	port := env("PORT", "8080")
	log.Printf("auth-service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
