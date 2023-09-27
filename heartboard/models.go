package heartboard

import (
	"time"
)

type Showcase struct {
	MessageID int64 `gorm:"primary_key"`
	GuildID   int64 `gorm:"index"`
	AuthorID  int64

	HeartBoardMessageID int64
	Approval            int64
	ImageUrl            string
	Title               string
	Content             string
	WebsiteUrl          string

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (w *Showcase) TableName() string {
	return "showcase"
}

type MemberQuote struct {
	MessageID      int64 `gorm:"primary_key"`
	QuoteTimestamp time.Time
	ChannelID      int64
	GuildID        int64 `gorm:"index"`
	AuthorID       int64

	StarBoardMessageID int64 `json:"star_board_message_id"`
	Approval           int64
	Content            string
	ImageUrl           string

	CreatedAt time.Time
	UpdatedAt time.Time
}
