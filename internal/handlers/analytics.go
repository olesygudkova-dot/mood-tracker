package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mood-tracker/internal/middleware"
)

type AnalyticsHandler struct {
	DB *pgxpool.Pool
}

// среднее настроение за период, лучший день, худший худший, количество дней с отметкой 
func (h *AnalyticsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)
	from, to := parsePeriod(r, 30)

	var avg float64
	var count, daysWithData int
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(ROUND(AVG(mood_score)::numeric, 1), 0),
		       COUNT(*),
		       COUNT(DISTINCT entry_date)
		FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date`,
		userID, from, to).Scan(&avg, &count, &daysWithData)

	var bestScore, worstScore int
	var bestDate, worstDate *time.Time
	_ = h.DB.QueryRow(r.Context(), `
		SELECT mood_score, entry_date FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date
		ORDER BY mood_score DESC, entry_date DESC LIMIT 1`,
		userID, from, to).Scan(&bestScore, &bestDate)
	_ = h.DB.QueryRow(r.Context(), `
		SELECT mood_score, entry_date FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date
		ORDER BY mood_score ASC, entry_date DESC LIMIT 1`,
		userID, from, to).Scan(&worstScore, &worstDate)

	resp := map[string]interface{}{
		"from":           from,
		"to":             to,
		"avg":            avg,
		"count":          count,
		"days_with_data": daysWithData,
		"best_score":     bestScore,
		"worst_score":    worstScore,
	}
	if bestDate != nil {
		resp["best_date"] = bestDate.Format("2006-01-02")
	}
	if worstDate != nil {
		resp["worst_date"] = worstDate.Format("2006-01-02")
	}
	writeJSON(w, resp)
}

// точки для графика: дата, среднее, количество

func (h *AnalyticsHandler) ByDay(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)
	from, to := parsePeriod(r, 30)

	rows, err := h.DB.Query(r.Context(), `
		SELECT entry_date,
		       ROUND(AVG(mood_score)::numeric, 1),
		       COUNT(*)
		FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date
		GROUP BY entry_date
		ORDER BY entry_date`, userID, from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type point struct {
		Date  string  `json:"date"`
		Label string  `json:"label"`
		Avg   float64 `json:"avg"`
		Count int     `json:"count"`
	}
	list := make([]point, 0)
	for rows.Next() {
		var d time.Time
		var p point
		if err := rows.Scan(&d, &p.Avg, &p.Count); err != nil {
			continue
		}
		p.Date = d.Format("2006-01-02")
		p.Label = d.Format("02.01")
		list = append(list, p)
	}
	writeJSON(w, list)
}

// среднее настроение по каждому тегу
func (h *AnalyticsHandler) ByTag(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)

	rows, err := h.DB.Query(r.Context(), `
		SELECT t.name, t.color,
		       ROUND(AVG(m.mood_score)::numeric, 1) AS avg,
		       COUNT(*) AS cnt
		FROM mood_entries m
		JOIN mood_tags mt ON m.id = mt.mood_entry_id
		JOIN tags t ON mt.tag_id = t.id
		WHERE m.user_id = $1
		GROUP BY t.name, t.color
		ORDER BY avg DESC`, userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type tagStat struct {
		Name  string  `json:"name"`
		Color string  `json:"color"`
		Avg   float64 `json:"avg"`
		Count int     `json:"count"`
	}
	list := make([]tagStat, 0)
	for rows.Next() {
		var s tagStat
		var color *string
		if err := rows.Scan(&s.Name, &color, &s.Avg, &s.Count); err != nil {
			continue
		}
		if color != nil {
			s.Color = *color
		}
		list = append(list, s)
	}
	writeJSON(w, list)
}

//среднее за суток 
func (h *AnalyticsHandler) ByHour(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)

	rows, err := h.DB.Query(r.Context(), `
		SELECT entry_hour,
		       ROUND(AVG(mood_score)::numeric, 1),
		       COUNT(*)
		FROM mood_entries
		WHERE user_id = $1 AND entry_hour IS NOT NULL
		GROUP BY entry_hour
		ORDER BY entry_hour`, userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type hourStat struct {
		Hour  int     `json:"hour"`
		Avg   float64 `json:"avg"`
		Count int     `json:"count"`
	}
	list := make([]hourStat, 0, 24)
	for rows.Next() {
		var s hourStat
		if err := rows.Scan(&s.Hour, &s.Avg, &s.Count); err != nil {
			continue
		}
		list = append(list, s)
	}
	writeJSON(w, list)
}

// среднее по дням недели
func (h *AnalyticsHandler) ByWeekday(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)

	rows, err := h.DB.Query(r.Context(), `
		SELECT EXTRACT(DOW FROM entry_date)::int AS dow,
		       ROUND(AVG(mood_score)::numeric, 1),
		       COUNT(*)
		FROM mood_entries
		WHERE user_id = $1
		GROUP BY dow
		ORDER BY dow`, userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	type dayStat struct {
		Dow   int     `json:"dow"`
		Name  string  `json:"name"`
		Avg   float64 `json:"avg"`
		Count int     `json:"count"`
	}
	names := []string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
	list := make([]dayStat, 0, 7)
	for rows.Next() {
		var s dayStat
		if err := rows.Scan(&s.Dow, &s.Avg, &s.Count); err != nil {
			continue
		}
		if s.Dow >= 0 && s.Dow < len(names) {
			s.Name = names[s.Dow]
		}
		list = append(list, s)
	}
	writeJSON(w, list)
}

// распределение записей по диапазонам оценок
func (h *AnalyticsHandler) Distribution(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)
	from, to := parsePeriod(r, 30)

	rows, err := h.DB.Query(r.Context(), `
		SELECT
			CASE
				WHEN mood_score BETWEEN 1 AND 3 THEN 'bad'
				WHEN mood_score BETWEEN 4 AND 6 THEN 'mid'
				WHEN mood_score BETWEEN 7 AND 8 THEN 'good'
				ELSE 'great'
			END AS bucket,
			COUNT(*)
		FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date
		GROUP BY bucket`, userID, from, to)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	result := map[string]int{"bad": 0, "mid": 0, "good": 0, "great": 0}
	total := 0
	for rows.Next() {
		var bucket string
		var cnt int
		if err := rows.Scan(&bucket, &cnt); err != nil {
			continue
		}
		result[bucket] = cnt
		total += cnt
	}
	writeJSON(w, map[string]interface{}{
		"bad":   result["bad"],
		"mid":   result["mid"],
		"good":  result["good"],
		"great": result["great"],
		"total": total,
	})
}

// сколько дней подряд ведётся дневник
func (h *AnalyticsHandler) Streak(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)

	rows, err := h.DB.Query(r.Context(), `
		SELECT DISTINCT entry_date
		FROM mood_entries
		WHERE user_id = $1
		ORDER BY entry_date DESC`, userID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var dates []time.Time
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			continue
		}
		dates = append(dates, d)
	}

	streak := 0
	if len(dates) > 0 {
		streak = 1
		for i := 1; i < len(dates); i++ {
			diff := dates[i-1].Sub(dates[i]).Hours() / 24
			if diff <= 1.5 && diff >= 0.5 {
				streak++
			} else {
				break
			}
		}
	}

	writeJSON(w, map[string]interface{}{
		"streak":     streak,
		"last_date":  dateOrEmpty(dates),
		"total_days": len(dates),
	})
}

//сравнение текущей недели с предыдущей
func (h *AnalyticsHandler) Compare(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	twoWeeksAgo := now.AddDate(0, 0, -14)

	avg := func(from, to time.Time) (float64, int) {
		var a float64
		var c int
		_ = h.DB.QueryRow(r.Context(), `
			SELECT COALESCE(ROUND(AVG(mood_score)::numeric, 1), 0), COUNT(*)
			FROM mood_entries
			WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date`,
			userID, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&a, &c)
		return a, c
	}

	curAvg, curCount := avg(weekAgo, now)
	prevAvg, prevCount := avg(twoWeeksAgo, weekAgo)

	diff := 0.0
	if curCount > 0 && prevCount > 0 {
		diff = curAvg - prevAvg
	}

	trend := "stable"
	if diff >= 1 {
		trend = "up"
	} else if diff <= -1 {
		trend = "down"
	}

	writeJSON(w, map[string]interface{}{
		"current_week_avg":   curAvg,
		"current_week_count": curCount,
		"prev_week_avg":      prevAvg,
		"prev_week_count":    prevCount,
		"diff":               diff,
		"trend":              trend,
	})
}

//топ-5 тегов в плохие и в хорошие дни
//  1 теги, встречающиеся в дни с оценкой≤4(плохие)
//2теги, встречающиеся в дни с оценкой ≥ 7(хорошие)
func (h *AnalyticsHandler) TopTags(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(int)

	fetch := func(cond string, order string) []map[string]interface{} {
		q := `
			SELECT t.name, COUNT(*) AS cnt
			FROM mood_entries m
			JOIN mood_tags mt ON m.id = mt.mood_entry_id
			JOIN tags t ON mt.tag_id = t.id
			WHERE m.user_id = $1 AND ` + cond + `
			GROUP BY t.name
			ORDER BY cnt ` + order + `
			LIMIT 5`
		rows, err := h.DB.Query(r.Context(), q, userID)
		if err != nil {
			return []map[string]interface{}{}
		}
		defer rows.Close()
		out := make([]map[string]interface{}, 0)
		for rows.Next() {
			var name string
			var cnt int
			if err := rows.Scan(&name, &cnt); err != nil {
				continue
			}
			out = append(out, map[string]interface{}{"name": name, "count": cnt})
		}
		return out
	}

	writeJSON(w, map[string]interface{}{
		"in_bad_days":  fetch("m.mood_score <= 4", "DESC"),
		"in_good_days": fetch("m.mood_score >= 7", "DESC"),
	})
}

//читает из URL, если пусто — последние N дней
func parsePeriod(r *http.Request, defaultDays int) (string, string) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		now := time.Now()
		from = now.AddDate(0, 0, -defaultDays).Format("2006-01-02")
		to = now.Format("2006-01-02")
	}
	return from, to
}

//дата в строку или пустая строка
func dateOrEmpty(dates []time.Time) string {
	if len(dates) == 0 {
		return ""
	}
	return dates[0].Format("2006-01-02")
}

// writeJSON - универсальная отправка JSON
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}