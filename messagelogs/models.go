package messagelogs

import (
	"github.com/cirelion/flint/lib/discordgo"
	"time"
)

type Attachment struct {
	ID        string `gorm:"primary_key"`
	MessageID int64

	Filename string
	Url      string
	ProxyUrl string
}
type Message struct {
	MessageID int64 `gorm:"primary_key"`
	ChannelID int64
	GuildID   int64 `gorm:"index"`
	AuthorID  int64

	MessageReferenceChannelID int64
	MessageReferenceID        int64
	DeletedByID               int64

	Content           string
	OriginalContent   string
	Attachments       []Attachment `gorm:"foreignKey:MessageID"`
	StickerID         int64
	StickerName       string
	StickerFormatType discordgo.StickerFormatType
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
