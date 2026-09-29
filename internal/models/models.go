package models

import "time"

type User struct {
	ID        int       `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type MoodEntry struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Date      string    `json:"date"`
	Hour      *int      `json:"hour"`
	Score     int       `json:"score"`
	Note      string    `json:"note"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

type Tag struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Advice struct {
	ID       int    `json:"id"`
	TagID    int    `json:"tag_id"`
	MinScore int    `json:"min_score"`
	MaxScore int    `json:"max_score"`
	Text     string `json:"text"`
}
