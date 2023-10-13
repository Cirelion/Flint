package games

import (
	"time"
)

type DuelState int

var (
	DuelInactive    DuelState = 0
	DuelAccepted    DuelState = 1
	DuelActive      DuelState = 2
	DuelMissedShot  DuelState = 3
	DuelMissedShots DuelState = 4
	DuelEnded       DuelState = 99
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

type Duel struct {
	ID           uint  `gorm:"primary_key"`
	ChallengerID int64 `json:"challenger_id"`
	ChallengedID int64 `json:"challenged_id"`
	WinnerID     int64 `json:"winner_id"`
	GuildID      int64 `gorm:"index"`

	DuelState DuelState `json:"duel_state"`
	Bet       int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (o Duel) TableName() string {
	return "duels"
}
