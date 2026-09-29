package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mood-tracker/internal/middleware"
)

type ChatHandler struct {
	DB *pgxpool.Pool
}

type chatRequest struct {
	Action  string `json:"action"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

type chatOption struct {
	Text  string `json:"text"`
	Value string `json:"value"`
}

type chatResponse struct {
	Message string       `json:"message"`
	Options []chatOption `json:"options"`
	Period  string       `json:"period,omitempty"`
	Date    string       `json:"date,omitempty"`
	Data    interface{}  `json:"data,omitempty"`
	Chart   interface{}  `json:"chart,omitempty"`
}

type moodPoint struct {
	Date  string  `json:"date"`
	Label string  `json:"label"`
	Avg   float64 `json:"avg"`
	Count int     `json:"count"`
}
type dayPoint struct {
	Hour  int     `json:"hour"`
	Score float64 `json:"score"`
	Note  string  `json:"note,omitempty"`
}

type periodStats struct {
	Avg          float64     `json:"avg"`
	Count        int         `json:"count"`
	DaysWithData int         `json:"days_with_data"`
	BestScore    int         `json:"best_score"`
	BestDate     string      `json:"best_date,omitempty"`
	WorstScore   int         `json:"worst_score"`
	WorstDate    string      `json:"worst_date,omitempty"`
	FirstAvg     *float64    `json:"first_avg,omitempty"`
	LastAvg      *float64    `json:"last_avg,omitempty"`
	Change       *float64    `json:"change,omitempty"`
	Trend        string      `json:"trend"`
	Points       []moodPoint `json:"points"`
}

// один день в списке выбора дня
type dayListItem struct {
	Date    string  `json:"date"`
	Label   string  `json:"label"`
	Avg     float64 `json:"avg"`
	Count   int     `json:"count"`
	Weekday string  `json:"weekday"`
}

// главный обработчик чата, роутит по action
func (h *ChatHandler) Respond(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(int)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	resp := chatResponse{}

	switch req.Action {
	// приветствие
	case "", "start":
		resp = chatResponse{
			Message: "Давай разберём твоё настроение. Я могу посмотреть конкретный день, неделю или месяц — не только среднее значение, но и изменения внутри дня.",
			Options: []chatOption{
				{Text: "Разобрать сегодня", Value: "day"},
				{Text: "Выбрать день", Value: "day_pick"},
				{Text: "Разобрать неделю", Value: "week"},
				{Text: "Разобрать месяц", Value: "month"},
				{Text: "Дать совет", Value: "advice"},
			},
		}

	// список дней с записями для выбора
	case "day_pick":
		days, err := h.getDaysList(r, userID, 30)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(days) == 0 {
			resp = chatResponse{
				Message: "Пока нет ни одной записи. Добавь настроение — и я смогу разобрать дни.",
				Options: []chatOption{{Text: "Назад", Value: "start"}},
			}
			break
		}
		opts := make([]chatOption, 0, len(days)+1)
		for _, d := range days {
			opts = append(opts, chatOption{
				Text:  fmt.Sprintf("%s (%s) — %.1f/10", d.Label, d.Weekday, d.Avg),
				Value: "day:" + d.Date,
			})
		}
		opts = append(opts, chatOption{Text: "Назад", Value: "start"})
		resp = chatResponse{
			Message: "Выбери день, который хочешь разобрать:",
			Options: opts,
			Data:    days,
		}

	// разбор конкретного дня (или сегодня, если дата не передана)
	case "day":
		date := req.Date
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		resp = h.buildDayResponse(r, userID, date)

	// переход к предыдущему/следующему дню с записями
	case "day_prev", "day_next":
		date := req.Date
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		var dir int
		if req.Action == "day_prev" {
			dir = -1
		} else {
			dir = 1
		}
		neighbor, err := h.getNeighborDay(r, userID, date, dir)
		if err != nil || neighbor == "" {
			resp = chatResponse{
				Message: "Больше нет дней с записями в эту сторону.",
				Options: []chatOption{
					{Text: "Выбрать день", Value: "day_pick"},
					{Text: "Назад", Value: "start"},
				},
				Date: date,
			}
			break
		}
		resp = h.buildDayResponse(r, userID, neighbor)

	// уточняющие вопросы по дню
	case "day_discuss":
		date := req.Date
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		points, err := h.getDayPoints(r, userID, date)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(points) == 0 {
			resp = chatResponse{Message: "За этот день нет данных.", Date: date, Options: []chatOption{{Text: "Назад", Value: "start"}}}
		} else if len(points) == 1 {
			resp = chatResponse{
				Message: "Что больше всего повлияло на твоё состояние в этот момент?",
				Options: []chatOption{
					{Text: "Работа / учёба", Value: "day_work:" + date},
					{Text: "Люди / отношения", Value: "day_people:" + date},
					{Text: "Усталость / сон", Value: "day_fatigue:" + date},
					{Text: "Что-то другое", Value: "day_other:" + date},
				},
				Date:   date,
				Period: "day",
			}
		} else {
			resp = chatResponse{
				Message: "В какой части дня произошло самое заметное изменение?",
				Options: []chatOption{
					{Text: "Утро", Value: "day_morning:" + date},
					{Text: "День", Value: "day_afternoon:" + date},
					{Text: "Вечер", Value: "day_evening:" + date},
					{Text: "Не могу определить", Value: "day_other:" + date},
				},
				Date:   date,
				Period: "day",
			}
		}

	case "day_work", "day_people", "day_fatigue", "day_other",
		"day_morning", "day_afternoon", "day_evening":
		date := req.Date
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		resp = chatResponse{
			Message: "Понял. Хочешь сравнить это с другими днями? Я могу посмотреть неделю и проверить, повторялось ли похожее изменение.",
			Options: []chatOption{
				{Text: "Да, посмотреть неделю", Value: "week"},
				{Text: "Нет, вернуться в начало", Value: "start"},
			},
			Date:   date,
			Period: "day",
		}
		//точки для графика дня
	case "day_chart":
		date := req.Date
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		points, err := h.getDayPoints(r, userID, date)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = chatResponse{
			Message: "Вот динамика отметок за этот день.",
			Options: []chatOption{
				{Text: "Обсудить день", Value: "day_discuss:" + date},
				{Text: "Неделя", Value: "week"},
				{Text: "Назад", Value: "start"},
			},
			Date:   date,
			Period: "day",
			Data:   points,
			Chart:  points,
		}

		//рзбор недели
	case "week":
		from := time.Now().AddDate(0, 0, -6)
		stats, err := h.getPeriodStats(r, userID, from, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = periodResponse("неделю", "week", stats, []chatOption{
			{Text: "Показать график", Value: "week_chart"},
			{Text: "Обсудить изменения", Value: "week_discuss"},
			{Text: "Выбрать день", Value: "day_pick"},
			{Text: "Месяц", Value: "month"},
			{Text: "Назад", Value: "start"},
		})

		//график недели
	case "week_chart":
		from := time.Now().AddDate(0, 0, -6)
		stats, err := h.getPeriodStats(r, userID, from, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = chatResponse{
			Message: "График показывает среднее настроение по каждому дню, в котором были записи.",
			Options: []chatOption{
				{Text: "Обсудить график", Value: "week_discuss"},
				{Text: "Выбрать день", Value: "day_pick"},
				{Text: "Назад", Value: "start"},
			},
			Period: "week",
			Data:   stats,
			Chart:  stats.Points,
		}

	// \обсуждение недели
	case "week_discuss":
		from := time.Now().AddDate(0, 0, -6)
		stats, err := h.getPeriodStats(r, userID, from, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = chatResponse{
			Message: weekDiscussion(stats),
			Options: []chatOption{
				{Text: "Показать график", Value: "week_chart"},
				{Text: "Выбрать день", Value: "day_pick"},
				{Text: "Месяц", Value: "month"},
				{Text: "Назад", Value: "start"},
			},
			Period: "week",
			Data:   stats,
		}

	//разбор месяца
	case "month":
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		stats, err := h.getPeriodStats(r, userID, from, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = periodResponse("месяц", "month", stats, []chatOption{
			{Text: "Показать график", Value: "month_chart"},
			{Text: "Обсудить результаты", Value: "month_discuss"},
			{Text: "Выбрать день", Value: "day_pick"},
			{Text: "Неделя", Value: "week"},
			{Text: "Назад", Value: "start"},
		})

	//график месяца
	case "month_chart":
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		stats, err := h.getPeriodStats(r, userID, from, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = chatResponse{
			Message: "График показывает, как менялось среднее настроение по дням текущего месяца.",
			Options: []chatOption{
				{Text: "Обсудить результаты", Value: "month_discuss"},
				{Text: "Выбрать день", Value: "day_pick"},
				{Text: "Неделя", Value: "week"},
				{Text: "Назад", Value: "start"},
			},
			Period: "month",
			Data:   stats,
			Chart:  stats.Points,
		}

	//обсуждение месяца
	case "month_discuss":
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		stats, err := h.getPeriodStats(r, userID, from, now)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp = chatResponse{
			Message: monthDiscussion(stats),
			Options: []chatOption{
				{Text: "Показать график", Value: "month_chart"},
				{Text: "Выбрать день", Value: "day_pick"},
				{Text: "Посмотреть неделю", Value: "week"},
				{Text: "Назад", Value: "start"},
			},
			Period: "month",
			Data:   stats,
		}

	//свободный текст от пользователя — простой разбор по ключевым словам
	case "message":
		text := normalize(req.Message)
		if text == "" {
			resp = chatResponse{
				Message: "Напиши, что произошло или что ты почувствовал(а), и я продолжу разбор.",
				Options: []chatOption{{Text: "Разобрать день", Value: "day"}},
			}
			break
		}

		if containsAny(text, "хуже", "плохо", "тяжело", "устал", "устала", "тревог", "груст", "разозл", "бесит", "поссор", "стресс") {
			resp = chatResponse{
				Message: "Похоже, в твоём ответе есть момент, который заметно повлиял на состояние. Что было главным: сама ситуация, другой человек или накопившаяся усталость?",
				Options: []chatOption{
					{Text: "Сама ситуация", Value: "day_other"},
					{Text: "Другой человек", Value: "day_people"},
					{Text: "Усталость", Value: "day_fatigue"},
					{Text: "Не знаю", Value: "week"},
				},
			}
		} else if containsAny(text, "хорош", "лучше", "рад", "счаст", "прият", "весел", "отдохнул", "отдохнула") {
			resp = chatResponse{
				Message: "Похоже, это скорее положительный момент. Хочешь проверить, повторяются ли похожие хорошие дни или события в течение недели?",
				Options: []chatOption{
					{Text: "Да, посмотреть неделю", Value: "week"},
					{Text: "Разобрать сегодняшний день", Value: "day"},
					{Text: "Вернуться в начало", Value: "start"},
				},
			}
		} else {
			resp = chatResponse{
				Message: "Понял. Давай уточним контекст: это скорее повлияло на настроение в лучшую сторону, в худшую или почти никак?",
				Options: []chatOption{
					{Text: "Стало лучше", Value: "day_other"},
					{Text: "Стало хуже", Value: "day_other"},
					{Text: "Почти не повлияло", Value: "week"},
				},
			}
		}

	//случайный совет из БД
	case "advice":
		var advice string
		err := h.DB.QueryRow(r.Context(), `SELECT text FROM advice ORDER BY RANDOM() LIMIT 1`).Scan(&advice)
		if err != nil {
			advice = "Попробуй сделать паузу, спокойно выдохнуть и дать себе немного времени без новых задач."
		}
		resp = chatResponse{
			Message: advice,
			Options: []chatOption{
				{Text: "Ещё совет", Value: "advice"},
				{Text: "Разобрать день", Value: "day"},
				{Text: "Назад", Value: "start"},
			},
		}

	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}

// собирает ответ по конкретному дню: точки, навигация ← →, кнопки
func (h *ChatHandler) buildDayResponse(r *http.Request, userID int, date string) chatResponse {
	points, err := h.getDayPoints(r, userID, date)
	if err != nil {
		return chatResponse{Message: "Ошибка загрузки данных за день.", Date: date}
	}

	pretty := prettyDate(date)

	if len(points) == 0 {
		return chatResponse{
			Message: fmt.Sprintf("За %s пока нет отметок настроения. Добавь хотя бы одну — тогда я смогу разобрать день.", pretty),
			Options: []chatOption{
				{Text: "Выбрать день", Value: "day_pick"},
				{Text: "Назад", Value: "start"},
			},
			Date:   date,
			Period: "day",
		}
	}

	prevDate, _ := h.getNeighborDay(r, userID, date, -1)
	nextDate, _ := h.getNeighborDay(r, userID, date, 1)

	nav := []chatOption{}
	if prevDate != "" {
		nav = append(nav, chatOption{Text: "← " + prevDate, Value: "day:" + prevDate})
	}
	if nextDate != "" {
		nav = append(nav, chatOption{Text: nextDate + " →", Value: "day:" + nextDate})
	}

	if len(points) == 1 {
		p := points[0]
		opts := append(nav, []chatOption{
			{Text: "Обсудить эту отметку", Value: "day_discuss:" + date},
			{Text: "Выбрать другой день", Value: "day_pick"},
			{Text: "Назад", Value: "start"},
		}...)
		return chatResponse{
			Message: fmt.Sprintf("%s: одна отметка — %s, %.0f/10. По одной отметке динамику не построить, но можем обсудить именно это состояние.", pretty, hourText(p.Hour), p.Score),
			Options: opts,
			Date:    date,
			Period:  "day",
			Data:    points,
		}
	}

	message := pretty + ". " + describeDay(points)
	opts := append(nav, []chatOption{
		{Text: "Обсудить, что изменилось", Value: "day_discuss:" + date},
		{Text: "Посмотреть график дня", Value: "day_chart:" + date},
		{Text: "Выбрать другой день", Value: "day_pick"},
		{Text: "Назад", Value: "start"},
	}...)

	return chatResponse{
		Message: message,
		Options: opts,
		Date:    date,
		Period:  "day",
		Data:    points,
		Chart:   points,
	}
}

// список последних N дней с записями (для выбора)
func (h *ChatHandler) getDaysList(r *http.Request, userID int, limit int) ([]dayListItem, error) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT entry_date,
		       ROUND(AVG(mood_score)::numeric, 1),
		       COUNT(*)
		FROM mood_entries
		WHERE user_id = $1
		GROUP BY entry_date
		ORDER BY entry_date DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]dayListItem, 0)
	for rows.Next() {
		var d time.Time
		var item dayListItem
		if err := rows.Scan(&d, &item.Avg, &item.Count); err != nil {
			return nil, err
		}
		item.Date = d.Format("2006-01-02")
		item.Label = d.Format("02.01.2006")
		item.Weekday = weekdayRu(d)
		list = append(list, item)
	}
	return list, rows.Err()
}

func (h *ChatHandler) getNeighborDay(r *http.Request, userID int, date string, dir int) (string, error) {
	var sql string
	if dir < 0 {
		sql = `SELECT MAX(entry_date) FROM mood_entries WHERE user_id = $1 AND entry_date < $2::date`
	} else {
		sql = `SELECT MIN(entry_date) FROM mood_entries WHERE user_id = $1 AND entry_date > $2::date`
	}
	var d *time.Time
	err := h.DB.QueryRow(r.Context(), sql, userID, date).Scan(&d)
	if err != nil || d == nil {
		return "", err
	}
	return d.Format("2006-01-02"), nil
}

// все точки за один день:час - оценка - заметка
func (h *ChatHandler) getDayPoints(r *http.Request, userID int, date string) ([]dayPoint, error) {
	rows, err := h.DB.Query(r.Context(), `
		SELECT entry_hour, mood_score, COALESCE(note, '')
		FROM mood_entries
		WHERE user_id = $1 AND entry_date = $2 AND entry_hour IS NOT NULL
		ORDER BY entry_hour`, userID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]dayPoint, 0)
	for rows.Next() {
		var p dayPoint
		if err := rows.Scan(&p.Hour, &p.Score, &p.Note); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// статистика за пери
func (h *ChatHandler) getPeriodStats(r *http.Request, userID int, from, to time.Time) (periodStats, error) {
	stats := periodStats{Points: make([]moodPoint, 0)}

	// среднее по дням
	rows, err := h.DB.Query(r.Context(), `
		SELECT entry_date,
		       ROUND(AVG(mood_score)::numeric, 1),
		       COUNT(*)
		FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date
		GROUP BY entry_date
		ORDER BY entry_date`, userID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	var sum float64
	for rows.Next() {
		var date time.Time
		var p moodPoint
		if err := rows.Scan(&date, &p.Avg, &p.Count); err != nil {
			return stats, err
		}
		p.Date = date.Format("2006-01-02")
		p.Label = date.Format("02.01")
		stats.Points = append(stats.Points, p)
		sum += p.Avg * float64(p.Count)
		stats.Count += p.Count
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}

	stats.DaysWithData = len(stats.Points)
	if stats.Count > 0 {
		stats.Avg = sum / float64(stats.Count)
	}
	if len(stats.Points) > 0 {
		stats.FirstAvg = &stats.Points[0].Avg
		last := stats.Points[len(stats.Points)-1].Avg
		stats.LastAvg = &last
		change := last - stats.Points[0].Avg
		stats.Change = &change
		stats.Trend = trendName(change)
	}

	// лучший день
	var bestScore int
	var bestDate time.Time
	if err := h.DB.QueryRow(r.Context(), `
		SELECT mood_score, entry_date
		FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date
		ORDER BY mood_score DESC, entry_date DESC
		LIMIT 1`, userID, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&bestScore, &bestDate); err == nil {
		stats.BestScore = bestScore
		stats.BestDate = bestDate.Format("02.01")
	}

	// худший день
	var worstScore int
	var worstDate time.Time
	if err := h.DB.QueryRow(r.Context(), `
		SELECT mood_score, entry_date
		FROM mood_entries
		WHERE user_id = $1 AND entry_date BETWEEN $2::date AND $3::date
		ORDER BY mood_score ASC, entry_date DESC
		LIMIT 1`, userID, from.Format("2006-01-02"), to.Format("2006-01-02")).Scan(&worstScore, &worstDate); err == nil {
		stats.WorstScore = worstScore
		stats.WorstDate = worstDate.Format("02.01")
	}

	return stats, nil
}

// текстовый ответ по периоду
func periodResponse(name, period string, s periodStats, options []chatOption) chatResponse {
	if s.Count == 0 {
		return chatResponse{
			Message: fmt.Sprintf("За %s пока нет записей. Когда появятся отметки, я смогу разобрать динамику.", name),
			Options: options,
			Period:  period,
			Data:    s,
		}
	}

	message := fmt.Sprintf("За %s: %d отметок за %d дней с данными, среднее настроение %.1f/10.", name, s.Count, s.DaysWithData, s.Avg)
	if s.Change != nil && s.DaysWithData >= 2 {
		message += " " + trendText(*s.Change, s.Trend)
	}
	if s.BestDate != "" && s.WorstDate != "" && s.BestDate != s.WorstDate {
		message += fmt.Sprintf(" Самая высокая оценка — %d/10 (%s), самая низкая — %d/10 (%s).", s.BestScore, s.BestDate, s.WorstScore, s.WorstDate)
	}
	return chatResponse{Message: message, Options: options, Period: period, Data: s}
}

// описание динамики внутри дня
func describeDay(points []dayPoint) string {
	if len(points) < 2 {
		return "Недостаточно данных для сравнения."
	}

	sorted := append([]dayPoint(nil), points...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Hour < sorted[j].Hour })
	first, last := sorted[0], sorted[len(sorted)-1]
	delta := last.Score - first.Score

	if delta <= -2 {
		return fmt.Sprintf("Похоже, день закончился заметно хуже, чем начался: %s — %.0f/10, а %s — %.0f/10. Разница %.0f пункта. Похоже на ухудшение состояния во второй части дня.", hourText(first.Hour), first.Score, hourText(last.Hour), last.Score, -delta)
	}
	if delta >= 2 {
		return fmt.Sprintf("Похоже, к концу дня тебе стало лучше: %s — %.0f/10, а %s — %.0f/10. Улучшение на %.0f пункта.", hourText(first.Hour), first.Score, hourText(last.Hour), last.Score, delta)
	}
	return fmt.Sprintf("Настроение в течение дня было относительно стабильным: от %.0f/10 до %.0f/10. Разница — %.0f пункта.", first.Score, last.Score, abs(delta))
}

// текст обсуждения недели
func weekDiscussion(s periodStats) string {
	if s.Count == 0 {
		return "За эту неделю пока недостаточно данных для обсуждения."
	}
	if s.Change != nil && *s.Change <= -2 {
		return fmt.Sprintf("За неделю заметно снижение: первая отметка в среднем %.1f, последняя %.1f. Самая низкая оценка была %d/10 (%s). Можно разобрать этот день отдельно и посмотреть, что происходило внутри него.", value(s.FirstAvg), value(s.LastAvg), s.WorstScore, s.WorstDate)
	}
	if s.Change != nil && *s.Change >= 2 {
		return fmt.Sprintf("За неделю есть заметное улучшение: от %.1f в начале до %.1f к концу. Самая высокая оценка — %d/10 (%s). Можно разобрать этот день подробнее.", value(s.FirstAvg), value(s.LastAvg), s.BestScore, s.BestDate)
	}
	return fmt.Sprintf("В течение недели настроение без большого общего сдвига: среднее %.1f/10. Самая низкая точка — %d/10 (%s), самая высокая — %d/10 (%s).", s.Avg, s.WorstScore, s.WorstDate, s.BestScore, s.BestDate)
}

// текст обсуждения месяца
func monthDiscussion(s periodStats) string {
	if s.Count == 0 {
		return "За текущий месяц пока нет данных для обсуждения."
	}
	if s.Change != nil && *s.Change <= -2 {
		return fmt.Sprintf("В этом месяце начало периода выглядит лучше конца: %.1f → %.1f. Это не говорит о причине само по себе, но такой перепад стоит посмотреть подробнее. Самая низкая оценка — %d/10 (%s).", value(s.FirstAvg), value(s.LastAvg), s.WorstScore, s.WorstDate)
	}
	if s.Change != nil && *s.Change >= 2 {
		return fmt.Sprintf("В этом месяце настроение в целом двигалось вверх: %.1f → %.1f. Самая высокая оценка — %d/10 (%s). Можно посмотреть, какие записи были в этот день.", value(s.FirstAvg), value(s.LastAvg), s.BestScore, s.BestDate)
	}
	return fmt.Sprintf("В этом месяце среднее настроение %.1f/10. Диапазон наблюдений — от %d/10 до %d/10. Если хочешь, можно перейти к неделе и разобрать изменения подробнее.", s.Avg, s.WorstScore, s.BestScore)
}

// определяет тренд по разнице
func trendName(delta float64) string {
	switch {
	case delta >= 1:
		return "up"
	case delta <= -1:
		return "down"
	default:
		return "stable"
	}
}

// текст про тренд
func trendText(delta float64, trend string) string {
	switch trend {
	case "up":
		return fmt.Sprintf("От начала к концу периода есть рост примерно на %.1f пункта.", delta)
	case "down":
		return fmt.Sprintf("От начала к концу периода есть снижение примерно на %.1f пункта.", -delta)
	default:
		return "Большого изменения между началом и концом периода нет."
	}
}

// приводит строку к нижнему регистру и заменяет ё на е
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.ReplaceAll(s, "ё", "е")
}

// проверяет, есть ли в тексте одно из слов
func containsAny(s string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(s, word) {
			return true
		}
	}
	return false
}

// час в формат
func hourText(hour int) string {
	return fmt.Sprintf("%02d:00", hour)
}

// модуль числа
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func value(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
func prettyDate(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	months := []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

// вс, пн, вт, ...
func weekdayRu(t time.Time) string {
	days := []string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
	return days[t.Weekday()]
}
