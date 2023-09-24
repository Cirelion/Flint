package heartboard

import (
	"time"
)

type Showcase struct {
	MessageID int64 `gorm:"primary_key"`
	GuildID   int64 `gorm:"index"`
	AuthorID  int64

	Approval            int
	ImageUrl            string
	Title               string
	Content             string
	WebsiteUrl          string
	HeartBoardMessageID int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (w *Showcase) TableName() string {
	return "showcase"
}
