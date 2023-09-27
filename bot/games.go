package bot

import (
	"github.com/botlabs-gg/yagpdb/v2/common"
	"time"
)

func screwTracker() {
	ticker := time.NewTicker(time.Hour * 2)
	for {
		<-ticker.C
		resetScrewCount()
	}
}

type Player struct {
	UserID  uint64 `gorm:"primary_key" json:"user_id"`
	GuildID int64  `gorm:"index" json:"guild_id"`

	Initialized    bool
	ScrewCount     int64 `gorm:"default:50"`
	ScrewsGiven    int64
	ScrewsReceived int64

	LastScrewCheck time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (o Player) TableName() string {
	return "players"
}

func resetScrewCount() {
	rows, err := common.GORM.Table("players").Rows()
	if err != nil {
		logger.Error(err)
		return
	}

	for rows.Next() {
		var player Player
		err = common.GORM.ScanRows(rows, &player)
		if err != nil {
			logger.Error(err)
			return
		}

		if time.Since(player.LastScrewCheck) > time.Hour*12 {
			err = common.GORM.Table("players").Where("user_id = ?", player.UserID).Update("last_screw_check", time.Now()).Error

			if player.ScrewCount < 50 {
				err = common.GORM.Table("players").Where("user_id = ?", player.UserID).Update("screw_count", 50).Error
				if err != nil {
					logger.Error(err)
					return
				}
				err = SendDM(int64(player.UserID), "Your screws have been reset to 50 again! Have fun betting!")
			}
		}
	}

	err = rows.Close()
	if err != nil {
		logger.Error(err)
		return
	}
}
