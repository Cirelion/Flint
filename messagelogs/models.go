package messagelogs

import (
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
	"time"
)

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
	AttachmentUrl     string
	StickerID         int64
	StickerName       string
	StickerFormatType discordgo.StickerFormatType
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
