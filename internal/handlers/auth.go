package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"mood-tracker/internal/auth"
	"mood-tracker/internal/config"
)

type AuthHandler struct {
	DB  *pgxpool.Pool
	Cfg *config.Config
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "hash error", 500)
		return
	}

	var userID int
	err = h.DB.QueryRow(r.Context(),
		`INSERT INTO users (email, password_hash, name) VALUES ($1, $2, $3) RETURNING id`,
		req.Email, string(hash), req.Name,
	).Scan(&userID)
	if err != nil {
		http.Error(w, "user exists or db error", 400)
		return
	}

	token, _ := auth.GenerateToken(userID, h.Cfg.JWTSecret)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"user_id": userID,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}

	var userID int
	var hash string
	err := h.DB.QueryRow(r.Context(),
		`SELECT id, password_hash FROM users WHERE email = $1`,
		req.Email,
	).Scan(&userID, &hash)
	if err != nil {
		http.Error(w, "invalid credentials", 401)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		http.Error(w, "invalid credentials", 401)
		return
	}

	token, _ := auth.GenerateToken(userID, h.Cfg.JWTSecret)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"user_id": userID,
	})
}
