package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"mood-tracker/internal/middleware"
	"mood-tracker/internal/models"
)

type MoodHandler struct {
	DB *pgxpool.Pool
}

func (h *MoodHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)

	var req struct {
		Date  string   `json:"date"`
		Hour  *int     `json:"hour"`
		Score int      `json:"score"`
		Note  string   `json:"note"`
		Tags  []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", 400)
		return
	}

	var entryID int
	err := h.DB.QueryRow(r.Context(),
		`INSERT INTO mood_entries (user_id, entry_date, entry_hour, mood_score, note)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, req.Date, req.Hour, req.Score, req.Note,
	).Scan(&entryID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, tagName := range req.Tags {
		var tagID int
		_ = h.DB.QueryRow(r.Context(), `SELECT id FROM tags WHERE name = $1`, tagName).Scan(&tagID)
		if tagID > 0 {
			_, _ = h.DB.Exec(r.Context(),
				`INSERT INTO mood_tags (mood_entry_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				entryID, tagID)
		}
	}

	json.NewEncoder(w).Encode(map[string]int{"id": entryID})
}

func (h *MoodHandler) GetByDate(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)
	date := chi.URLParam(r, "date")

	rows, err := h.DB.Query(r.Context(),
		`SELECT id, entry_date, entry_hour, mood_score, COALESCE(note, '')
		 FROM mood_entries WHERE user_id = $1 AND entry_date = $2
		 ORDER BY entry_hour NULLS FIRST`,
		userID, date)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var entries []models.MoodEntry
	for rows.Next() {
		var e models.MoodEntry
		var d time.Time
		if err := rows.Scan(&e.ID, &d, &e.Hour, &e.Score, &e.Note); err != nil {
			continue
		}
		e.Date = d.Format("2006-01-02")
		entries = append(entries, e)
	}

	json.NewEncoder(w).Encode(entries)
}

func (h *MoodHandler) Calendar(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)
	month := chi.URLParam(r, "month")

	rows, err := h.DB.Query(r.Context(),
		`SELECT entry_date, ROUND(AVG(mood_score)::numeric, 1), COUNT(*)
		 FROM mood_entries
		 WHERE user_id = $1 AND TO_CHAR(entry_date, 'YYYY-MM') = $2
		 GROUP BY entry_date ORDER BY entry_date`,
		userID, month)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type Day struct {
		Date  string  `json:"date"`
		Avg   float64 `json:"avg"`
		Count int     `json:"count"`
	}
	var days []Day
	for rows.Next() {
		var d Day
		var t time.Time
		if err := rows.Scan(&t, &d.Avg, &d.Count); err != nil {
			continue
		}
		d.Date = t.Format("2006-01-02")
		days = append(days, d)
	}

	json.NewEncoder(w).Encode(days)
}

func (h *MoodHandler) Tags(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(r.Context(), `SELECT id, name, COALESCE(color, '') FROM tags ORDER BY name`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	json.NewEncoder(w).Encode(tags)
}
