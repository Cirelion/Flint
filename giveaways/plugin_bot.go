package giveaways

import (
	"github.com/AlekSi/pointer"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/discordgo"
	"math/rand"
	"time"
)

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Giveaways",
		SysName:  "giveaways",
		Category: common.PluginCategoryMisc,
	}
}

var logger = common.GetPluginLogger(&Plugin{})

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})
	common.GORM.AutoMigrate(&Giveaway{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	rows, err := common.GORM.Table("giveaways").Where("active = ?", common.BoolToPointer(true)).Rows()
	if err != nil {
		logger.Error(err)
		return
	}

	for rows.Next() {
		var giveaway Giveaway
		err = common.GORM.ScanRows(rows, &giveaway)
		if err != nil {
			logger.Error(err)
			return
		}

		go giveawayTracker(giveaway)
	}

	err = rows.Close()
	if err != nil {
		logger.Error(err)
		return
	}
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		StartGiveaway,
		CancelGiveaway,
		RerollGiveaway,
	)
}

func giveawayTracker(giveaway Giveaway) {
	ticker := time.NewTicker(time.Second * 30)
	for {
		<-ticker.C
		if time.Now().After(giveaway.EndsAt) {
			msg, err := handleEndGiveaway(giveaway)
			if msg != "" {
				logger.Println(msg)
			}

			if err != nil {
				logger.Error(err)
			}

			ticker.Stop()
		}
	}
}

func handleEndGiveaway(giveaway Giveaway) (string, error) {
	if !pointer.GetBool(giveaway.Active) {
		return "Giveaway is already inactive", nil
	}

	msg, err := common.BotSession.ChannelMessage(giveaway.ChannelID, giveaway.MessageID)

	if err != nil {
		if pointer.GetBool(giveaway.Active) {
			common.GORM.Model(&giveaway).Updates([]interface{}{Giveaway{Active: common.BoolToPointer(false)}})
		}

		return "Message doesn't exist anymore", nil
	}

	reactions, err := common.BotSession.MessageReactions(giveaway.ChannelID, giveaway.MessageID, "🎉", 0, 0, 0)

	if len(reactions) > 0 {
		for i := int64(0); i < giveaway.MaxWinners; i++ {
			if len(reactions) == 0 {
				break
			}

			randomIndex := rand.Intn(len(reactions))
			if !reactions[randomIndex].Bot {
				giveaway.Winners = append(giveaway.Winners, reactions[randomIndex].ID)
			}
			reactions = removeUser(reactions, randomIndex)
		}
	}

	for _, reaction := range reactions {
		if !reaction.Bot {
			giveaway.Participants = append(giveaway.Participants, reaction.ID)
		}
	}

	embed := msg.Embeds[0]
	embed.Timestamp = time.Now().Format(time.RFC3339)
	embed.Title = "Giveaway has ended"
	embed.Description, _ = generateGiveawayDescription(giveaway)
	embed.Color = 12257822
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Giveaway ended"}
	_, err = common.BotSession.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:      giveaway.MessageID,
		Channel: giveaway.ChannelID,
		Embeds:  []*discordgo.MessageEmbed{embed},
	})

	if err != nil {
		return "", err
	}

	common.GORM.Model(&giveaway).Updates([]interface{}{Giveaway{Participants: giveaway.Participants, Winners: giveaway.Winners, Active: common.BoolToPointer(false)}})

	return "Giveaway ended", nil
}
