package applications

//type Giveaway struct {
//	MessageID int64 `gorm:"primary_key"`
//	ChannelID int64
//	UserID    int64 `json:"user_id"`
//	GuildID   int64 `gorm:"index"`
//
//	Active       *bool `sql:"DEFAULT:true"`
//	Prize        string
//	MaxWinners   int64
//	Participants pq.Int64Array `gorm:"type:bigint[]"`
//	Winners      pq.Int64Array `gorm:"type:bigint[]"`
//
//	EndsAt    time.Time
//	CreatedAt time.Time
//	UpdatedAt time.Time
//}
//
//func (o Giveaway) TableName() string {
//	return "giveaways"
//}
