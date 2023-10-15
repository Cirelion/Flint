package bot

import (
	"fmt"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/common/configstore"
	"github.com/cirelion/flint/lib/discordgo"
	"strconv"
	"time"
)

type Config struct {
	configstore.GuildConfigModel
	//On Duty
	OnDutyRole                  string `valid:"role,true"`
	OnDutyChannelOne            string `valid:"channel,true"`
	OnDutyChannelOneDescription string `valid:"template,5000"`
	OnDutyChannelTwo            string `valid:"channel,true"`
	OnDutyChannelTwoDescription string `valid:"template,5000"`
}

func (c *Config) GetName() string {
	return "moderation"
}

func onDutyTracker() {
	ticker := time.NewTicker(time.Minute * 5)
	for {
		<-ticker.C
		updateOnDutyChannelDescriptions()
	}
}

type OnDuty struct {
	UserID  uint64 `gorm:"primary_key" json:"user_id"`
	GuildID int64  `gorm:"index" json:"guild_id"`

	OnDuty         bool
	OnDutyDuration time.Duration
	OnDutySetAt    time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (o OnDuty) TableName() string {
	return "on_duty"
}

func updateOnDutyChannelDescriptions() {
	guilds := State.GetShardGuilds(int64(common.BotSession.ShardID))
	rows, err := common.GORM.Table("on_duty").Rows()
	if err != nil {
		logger.Error(err)
		return
	}

	for _, guild := range guilds {
		firstRow := true
		conf := Config{}
		err = common.GORM.Table("moderation_configs").Where("guild_id = ?", guild.ID).First(&conf).Error
		if err != nil {
			logger.Error(err)
			return
		}

		OnDutyRole, _ := strconv.ParseInt(conf.OnDutyRole, 10, 64)
		onDutyChannelOne, _ := strconv.ParseInt(conf.OnDutyChannelOne, 10, 64)
		onDutyChannelTwo, _ := strconv.ParseInt(conf.OnDutyChannelTwo, 10, 64)
		memberList := " - Staff on duty: "

		if onDutyChannelOne != 0 {
			for rows.Next() {
				var onDuty OnDuty
				err = common.GORM.ScanRows(rows, &onDuty)
				if err != nil {
					logger.Error(err)
					continue
				}

				member, memberErr := GetMember(guild.ID, int64(onDuty.UserID))
				if memberErr != nil {
					logger.Error(memberErr)
					continue
				}

				if !onDuty.OnDuty && common.ContainsInt64Slice(member.Member.Roles, OnDutyRole) {
					err = common.GORM.Table("on_duty").Where("user_id = ?", member.User.ID).Update("on_duty", true).Error
					err = common.GORM.Table("on_duty").Where("user_id = ?", member.User.ID).Update("on_duty_set_at", time.Now()).Error
					onDuty.OnDuty = true

					if err != nil {
						logger.Error(err)
						continue
					}
				}

				if onDuty.OnDuty {
					if !common.ContainsInt64Slice(member.Member.Roles, OnDutyRole) {
						err = common.GORM.Table("on_duty").Where("user_id = ?", member.User.ID).Update("on_duty", false).Error
						continue
					} else if time.Since(onDuty.OnDutySetAt) > onDuty.OnDutyDuration {
						err = common.BotSession.GuildMemberRoleRemove(guild.ID, member.User.ID, OnDutyRole)
						err = common.GORM.Table("on_duty").Where("user_id = ?", member.User.ID).Update("on_duty", false).Error
						if err != nil {
							logger.Error(err)
							continue
						}

						err = SendDM(int64(onDuty.UserID), fmt.Sprintf("You have been automatically been set Off Duty after %s", common.HumanizeDuration(common.DurationPrecisionHours, onDuty.OnDutyDuration)))
						if err != nil {
							logger.Error(err)
							continue
						}
					} else {
						if firstRow {
							firstRow = false
							memberList += GetName(member)
						} else {
							memberList += ", " + GetName(member)
						}
					}
				}
			}

			err = rows.Close()
			if err != nil {
				continue
			}

			channel, channelErr := common.BotSession.Channel(onDutyChannelOne)
			if channelErr != nil {
				logger.Error(channelErr)
				continue
			}

			if memberList == " - Staff on duty: " {
				memberList += "None"
			}

			if channel.Topic != conf.OnDutyChannelOneDescription+memberList {
				_, channelErr = common.BotSession.ChannelEditComplex(onDutyChannelOne, &discordgo.ChannelEdit{
					Topic: conf.OnDutyChannelOneDescription + memberList,
				})
				if channelErr != nil {
					logger.Error(channelErr)
					continue
				}
				if onDutyChannelTwo != 0 {
					_, channelErr = common.BotSession.ChannelEditComplex(onDutyChannelTwo, &discordgo.ChannelEdit{
						Topic: conf.OnDutyChannelTwoDescription + memberList,
					})
					if channelErr != nil {
						logger.Error(channelErr)
						continue
					}
				}
			}
		}
	}
}
