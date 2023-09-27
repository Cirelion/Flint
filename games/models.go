package games

import (
	"time"
)

type Player struct {
	UserID  int64 `gorm:"primary_key" json:"user_id"`
	GuildID int64 `gorm:"index"`

	Initialized    bool
	ScrewCount     int64 `json:"screw_count"`
	ScrewsGiven    int64 `json:"screws_given"`
	ScrewsReceived int64 `json:"screws_received"`

	LastScrewCheck time.Time `json:"last_screw_check"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (o Player) TableName() string {
	return "players"
}
