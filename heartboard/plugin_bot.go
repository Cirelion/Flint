package heartboard

import (
	"fmt"
	"github.com/botlabs-gg/yagpdb/v2/bot"
	"github.com/botlabs-gg/yagpdb/v2/bot/eventsystem"
	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
	"github.com/botlabs-gg/yagpdb/v2/lib/dstate"
	"github.com/botlabs-gg/yagpdb/v2/moderation"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"regexp"
	"slices"
	"strings"
)

var (
	logger   = common.GetPluginLogger(&Plugin{})
	urlRegex = `https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*)`
)

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Heart board",
		SysName:  "heart_board",
		Category: common.PluginCategoryMisc,
	}
}

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})

	common.GORM.AutoMigrate(&Showcase{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, bot.ConcurrentEventHandler(handleReaction), eventsystem.EventMessageReactionAdd, eventsystem.EventMessageReactionRemove)
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		AddToHeartBoard,
	)
}

func handleReaction(evt *eventsystem.EventData) {
	var reaction *discordgo.MessageReaction

	switch e := evt.EvtInterface.(type) {
	case *discordgo.MessageReactionAdd:
		reaction = e.MessageReaction
	case *discordgo.MessageReactionRemove:
		reaction = e.MessageReaction
	}
	log.Println(reaction.Emoji.Name)
	if reaction.GuildID == 0 {
		return
	}

	channel := evt.GS.GetChannelOrThread(reaction.ChannelID)
	config, err := moderation.GetConfig(reaction.GuildID)
	if err != nil {
		logger.Error(err)
		return
	}

	if !isValidChannel(channel, config) {
		logger.Println("Invalid channel for heart board")
		return
	}

	showcase := &Showcase{
		MessageID: reaction.MessageID,
	}

	message, _ := common.BotSession.ChannelMessage(channel.ID, showcase.MessageID)
	err = common.GORM.Model(showcase).Find(showcase).Error
	showcase.Title = channel.Name
	showcase.Content = message.Content

	if err != nil {
		showcase.AuthorID = message.Author.ID
		showcase.GuildID = evt.GS.ID
		re, _ := regexp.Compile(urlRegex)
		if re.MatchString(message.Content) {
			showcase.WebsiteUrl = re.FindString(message.Content)
		}

		if len(message.Attachments) > 0 {
			showcase.ImageUrl = message.Attachments[0].URL
		}

		err = common.GORM.Model(showcase).Save(showcase).Error
		if err != nil {
			logger.Error(err)
		}
	}

	if len(message.Reactions) > 0 {
		showcase.Approval = handleCountApproval(message.Reactions)
	} else {
		showcase.Approval = 0
	}

	if err != nil {
		logger.Error(err)
		return
	}

	if showcase.Approval > 4 && showcase.ImageUrl != "" {
		embed := generateEmbed(showcase)
		if showcase.HeartBoardMessageID == 1 {
			msg, msgErr := common.BotSession.ChannelMessageSendEmbed(config.HeartBoardChannel, embed)

			if msgErr != nil {
				logger.Error(msgErr)
				return
			}
			showcase.HeartBoardMessageID = msg.ID
		} else {
			_, msgErr := common.BotSession.ChannelMessageEditEmbed(config.HeartBoardChannel, showcase.HeartBoardMessageID, embed)

			if msgErr != nil {
				logger.Error(msgErr)
				msg, _ := common.BotSession.ChannelMessageSendEmbed(config.HeartBoardChannel, embed)
				showcase.HeartBoardMessageID = msg.ID
			}
		}
	} else if showcase.HeartBoardMessageID != 1 {
		msgErr := common.BotSession.ChannelMessageDelete(config.HeartBoardChannel, showcase.HeartBoardMessageID)
		if msgErr != nil {
			logger.Error(msgErr)
		}
		showcase.HeartBoardMessageID = 1
	}

	err = common.GORM.Model(showcase).Update([]interface{}{showcase}).Error
	if err != nil {
		logger.Error(err)
	}
}

func generateEmbed(showcase *Showcase) *discordgo.MessageEmbed {
	member, err := bot.GetMember(showcase.GuildID, showcase.AuthorID)
	if err != nil {
		log.Error("memberErr: ", showcase.GuildID, showcase.AuthorID, err)
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Title:       showcase.Title,
		Description: strings.Replace(showcase.Content, showcase.WebsiteUrl, "", -1),
		URL:         showcase.WebsiteUrl,
		Color:       rand.Intn(16777215),
		Image: &discordgo.MessageEmbedImage{
			URL: showcase.ImageUrl,
		},
		Author: &discordgo.MessageEmbedAuthor{
			Name:    member.User.Username,
			IconURL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("❤️ %d | Created by: %s", showcase.Approval, member.User.Globalname),
		},
	}

	return embed
}

func handleCountApproval(reactions []*discordgo.MessageReactions) int {
	count := 0

	for _, reaction := range reactions {
		if reaction.Emoji.Name == "unhingedpinkheart" {
			count += reaction.Count
		}

		if reaction.Emoji.Name == "❌" {
			count -= reaction.Count
		}
	}

	return count
}

func isValidChannel(channel *dstate.ChannelState, config *moderation.Config) bool {
	channelID := channel.ID

	if channel.Type == 11 {
		channelID = channel.ParentID
	}

	if channelID == config.HeartBoardChannel || slices.Contains(config.ShowcaseChannels, channelID) {
		return true
	}

	return false
}
