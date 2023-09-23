package polls

import (
	"time"
)

type PollMessage struct {
	MessageID int64 `gorm:"primary_key"`
	GuildID   int64 `gorm:"index"`
	AuthorID  int64

	Question    string
	Votes       []Vote `gorm:"foreignKey:PollMessageID;references:MessageID"`
	Broken      bool
	IsStrawPoll bool
	Options     []SelectMenuOption `gorm:"foreignKey:PollMessageID;references:MessageID"`
	MaxOptions  int

	CreatedAt time.Time
	UpdatedAt time.Time
}

type SelectMenuOption struct {
	ID            int64 `gorm:"primary_key"`
	PollMessageID int64
	Label         string `json:"label,omitempty"`
	Value         string `json:"value"`
	EmojiName     string `json:"emoji_name"`
}

type Vote struct {
	UserID        int64 `gorm:"primary_key"`
	PollMessageID int64
	Vote          string
}
