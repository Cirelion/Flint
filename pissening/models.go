package pissening

import (
	"github.com/lib/pq"
	"time"
)

type PisseningState int

var (
	JoinID  = "pissening_join"
	StartID = "pissening_start"
	StopID  = "pissening_stop"
	PissID  = "pissening_piss"

	Initialized PisseningState = 0
	Started     PisseningState = 1
	Ended       PisseningState = 99
)

type Pisser struct {
	UserID  int64 `gorm:"primary_key" json:"user_id"`
	GuildID int64 `gorm:"index"`

	CompetedPissenings int64
	ActivePissening    *Pissening

	TotalPisses int64
	JarsSpilled int64
	HasPRole    *bool `json:"has_p_role"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (o Pisser) TableName() string {
	return "pissers"
}

type Pissening struct {
	ID        uint  `gorm:"primary_key"`
	GuildID   int64 `gorm:"index"`
	AuthorID  int64
	MessageID int64

	PisseningState PisseningState `json:"pissening_state"`
	Pissers        pq.Int64Array  `gorm:"type:bigint[]"`
	Pisses         pq.Int64Array  `gorm:"type:bigint[]"`
	JarSpiller     int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (o Pissening) TableName() string {
	return "pissenings"
}
