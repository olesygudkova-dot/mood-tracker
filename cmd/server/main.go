package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"mood-tracker/internal/config"
	"mood-tracker/internal/db"
	"mood-tracker/internal/handlers"
	"mood-tracker/internal/middleware"
)

func main() {
	cfg := config.Load()
	pool := db.Connect(cfg)
	defer pool.Close()

	authH := &handlers.AuthHandler{DB: pool, Cfg: cfg}
	moodH := &handlers.MoodHandler{DB: pool}
	analyticsH := &handlers.AnalyticsHandler{DB: pool}
	chatH := &handlers.ChatHandler{DB: pool}

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// CORS — до всех роутов
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if req.Method == "OPTIONS" {
				w.WriteHeader(200)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	r.Post("/api/auth/register", authH.Register)
	r.Post("/api/auth/login", authH.Login)

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.Auth(cfg.JWTSecret))

		r.Post("/moods", moodH.Create)
		r.Get("/moods/{date}", moodH.GetByDate)
		r.Get("/calendar/{month}", moodH.Calendar)
		r.Get("/tags", moodH.Tags)

		r.Get("/analytics/summary", analyticsH.Summary)
		r.Get("/analytics/by-tag", analyticsH.ByTag)
		r.Get("/analytics/by-hour", analyticsH.ByHour)
		r.Get("/analytics/by-weekday", analyticsH.ByWeekday)

		r.Post("/chat", chatH.Respond)
	})

	fs := http.FileServer(http.Dir("./static"))
	r.Handle("/*", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/" {
			http.ServeFile(w, req, "./static/index.html")
			return
		}
		fs.ServeHTTP(w, req)
	}))

	log.Printf("server on :%s", cfg.ServerPort)
	if err := http.ListenAndServe(":"+cfg.ServerPort, r); err != nil {
		log.Fatal(err)
	}
}
