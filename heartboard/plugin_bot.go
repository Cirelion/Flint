package heartboard

import (
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/bot/eventsystem"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	"github.com/cirelion/flint/moderation"
	"math/rand"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	logger   = common.GetPluginLogger(&Plugin{})
	urlRegex = `https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*)`
)

type Plugin struct {
	sync.RWMutex
}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Heart board",
		SysName:  "heart_board",
		Category: common.PluginCategoryMisc,
	}
}

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})

	common.GORM.AutoMigrate(&Showcase{}, &MemberQuote{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, p.handleReaction, eventsystem.EventMessageReactionAdd, eventsystem.EventMessageReactionRemove)
}

func (p *Plugin) handleReaction(evt *eventsystem.EventData) {
	var reaction *discordgo.MessageReaction

	switch e := evt.EvtInterface.(type) {
	case *discordgo.MessageReactionAdd:
		reaction = e.MessageReaction
	case *discordgo.MessageReactionRemove:
		reaction = e.MessageReaction
	}

	if reaction.GuildID == 0 {
		return
	}

	channel := evt.GS.GetChannelOrThread(reaction.ChannelID)
	config, err := moderation.GetConfig(reaction.GuildID)
	if err != nil {
		logger.Error(err)
		return
	}

	if isValidChannel(channel, config) {
		handleHeartBoard(config, channel, reaction)
	} else if reaction.Emoji.Name == "⭐" || reaction.Emoji.Name == "❌" {
		p.handleStarBoard(config, reaction)
	}
}

func initMemberQuote(guildID int64, message *discordgo.Message) *MemberQuote {
	quoteTimeStamp, err := message.Timestamp.Parse()
	if err != nil {
		logger.Error(err)
	}

	memberQuote := &MemberQuote{
		MessageID:      message.ID,
		ChannelID:      message.ChannelID,
		GuildID:        guildID,
		AuthorID:       message.Author.ID,
		QuoteTimestamp: quoteTimeStamp,
		Approval:       handleCountApproval(message.Reactions, "⭐"),
	}

	if message.Content != "" {
		memberQuote.Content = message.Content
	}

	if len(message.Attachments) > 0 {
		memberQuote.ImageUrl = message.Attachments[0].URL
	}

	err = common.GORM.Model(memberQuote).FirstOrCreate(memberQuote).Error
	if err != nil {
		logger.Error(err)
	}

	return memberQuote
}

func (p *Plugin) handleStarBoard(config *moderation.Config, reaction *discordgo.MessageReaction) {
	p.Lock()
	starboardEmoji := "⭐"
	message, err := common.BotSession.ChannelMessage(reaction.ChannelID, reaction.MessageID)
	approval := handleCountApproval(message.Reactions, "⭐")
	member, err := bot.GetMember(reaction.GuildID, reaction.UserID)
	if err != nil {
		logger.Error(err)
		p.Unlock()
		return
	}

	if !message.Author.Bot && (message.Content != "" || len(message.Attachments) > 0) {
		memberQuote := &MemberQuote{
			MessageID: reaction.MessageID,
		}

		err = common.GORM.Model(memberQuote).Find(memberQuote).Error
		if err != nil {
			memberQuote = initMemberQuote(reaction.GuildID, message)
		}

		if memberQuote.StarBoardMessageID > 1 {
			starboardMessage, starboardMessageErr := common.BotSession.ChannelMessage(config.StarBoardChannel, memberQuote.StarBoardMessageID)
			if starboardMessageErr != nil {
				logger.Error(starboardMessageErr)
				p.Unlock()
				return
			}

			reactedStars, reactionsErr := common.BotSession.MessageReactions(starboardMessage.ChannelID, starboardMessage.ID, "⭐", 0, 0, 0)
			reactedXs, reactionsErr := common.BotSession.MessageReactions(starboardMessage.ChannelID, starboardMessage.ID, "❌", 0, 0, 0)
			if reactionsErr != nil {
				logger.Error(reactionsErr)
				p.Unlock()
				return
			}

			hasUserReacted := false
			for _, user := range reactedStars {
				if user.ID == reaction.UserID {
					hasUserReacted = true
				}
			}

			for _, user := range reactedXs {
				if user.ID == reaction.UserID {
					hasUserReacted = true
				}
			}

			if hasUserReacted {
				err = common.BotSession.MessageReactionRemove(message.ChannelID, message.ID, reaction.Emoji.Name, reaction.UserID)
				if err != nil {
					logger.Error(err)
				}
				p.Unlock()
				return
			}

			approval = handleCountApproval(message.Reactions, "⭐") + handleCountApproval(starboardMessage.Reactions, "⭐")
		}

		err = common.GORM.Model(&memberQuote).Find(memberQuote).Error
		if err != nil {
			logger.Error(err)
			p.Unlock()
			return
		}

		if approval >= config.StarBoardThreshold {
			memberQuote.Approval = approval
			if memberQuote.StarBoardMessageID > 1 {
				if memberQuote.Approval >= config.StarBoardThreshold*2 {
					starboardEmoji = "🌟"
				}

				_, err = common.BotSession.ChannelMessageEditEmbed(config.StarBoardChannel, memberQuote.StarBoardMessageID, generateMemberQuoteEmbed(memberQuote, starboardEmoji))
				if err != nil {
					logger.Error(err)
					p.Unlock()
					return
				}
			} else {
				embedMessage, embedMessageErr := common.BotSession.ChannelMessageSendEmbed(config.StarBoardChannel, generateMemberQuoteEmbed(memberQuote, starboardEmoji))
				if embedMessageErr != nil {
					logger.Error(embedMessageErr)
					p.Unlock()
					return
				}

				err = common.BotSession.MessageReactionAdd(embedMessage.ChannelID, embedMessage.ID, "⭐")
				if err != nil {
					logger.Error(err)
				}

				err = common.BotSession.MessageReactionAdd(embedMessage.ChannelID, embedMessage.ID, "❌")
				if err != nil {
					logger.Error(err)
				}

				memberQuote.StarBoardMessageID = embedMessage.ID
			}
		} else if memberQuote.StarBoardMessageID > 1 {
			err = common.BotSession.ChannelMessageDelete(config.StarBoardChannel, memberQuote.StarBoardMessageID)
			if err != nil {
				logger.Error(err)
				p.Unlock()
				return
			}
			memberQuote.StarBoardMessageID = 1
		}

		err = common.GORM.Model(memberQuote).Update([]interface{}{memberQuote}).Error
		if err != nil {
			logger.Error(err)
			p.Unlock()
			return
		}
	} else if message.ChannelID == config.StarBoardChannel && !member.User.Bot {
		memberQuote := &MemberQuote{}
		err = common.GORM.Model(memberQuote).Where("star_board_message_id = ?", reaction.MessageID).Find(memberQuote).Error
		if err != nil {
			logger.Error(err)
			p.Unlock()
			return
		}

		msg, msgErr := common.BotSession.ChannelMessage(memberQuote.ChannelID, memberQuote.MessageID)
		if msgErr != nil {
			logger.Error(msgErr)
			p.Unlock()
			return
		}

		reactedStars, reactionsErr := common.BotSession.MessageReactions(memberQuote.ChannelID, memberQuote.MessageID, "⭐", 0, 0, 0)
		reactedXs, reactionsErr := common.BotSession.MessageReactions(memberQuote.ChannelID, memberQuote.MessageID, "❌", 0, 0, 0)
		if reactionsErr != nil {
			logger.Error(reactionsErr)
			p.Unlock()
			return
		}

		hasUserReacted := false
		for _, user := range reactedStars {
			if user.ID == reaction.UserID {
				hasUserReacted = true
			}
		}

		for _, user := range reactedXs {
			if user.ID == reaction.UserID {
				hasUserReacted = true
			}
		}

		if hasUserReacted {
			err = common.BotSession.MessageReactionRemove(message.ChannelID, memberQuote.StarBoardMessageID, reaction.Emoji.Name, reaction.UserID)
			if err != nil {
				logger.Error(err)
				p.Unlock()
				return
			}
		}

		memberQuote.Approval = handleCountApproval(msg.Reactions, "⭐") + handleCountApproval(message.Reactions, "⭐")
		_, err = common.BotSession.ChannelMessageEditEmbed(config.StarBoardChannel, memberQuote.StarBoardMessageID, generateMemberQuoteEmbed(memberQuote, starboardEmoji))
		err = common.GORM.Model(memberQuote).Update([]interface{}{memberQuote}).Error
		if err != nil {
			logger.Error(err)
			p.Unlock()
			return
		}
	}
	p.Unlock()
}

func handleHeartBoard(config *moderation.Config, channel *dstate.ChannelState, reaction *discordgo.MessageReaction) {
	showcase := &Showcase{
		MessageID: reaction.MessageID,
	}

	message, _ := common.BotSession.ChannelMessage(channel.ID, showcase.MessageID)
	err := common.GORM.Model(showcase).Find(showcase).Error
	showcase.Title = channel.Name
	showcase.Content = message.Content

	if err != nil {
		showcase.AuthorID = message.Author.ID
		showcase.GuildID = reaction.GuildID
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
		showcase.Approval = handleCountApproval(message.Reactions, "unhingedpinkheart")
	} else {
		showcase.Approval = 0
	}

	if err != nil {
		logger.Error(err)
		return
	}

	if showcase.Approval >= config.HeartBoardThreshold && showcase.ImageUrl != "" {
		embed := generateShowcaseEmbed(showcase)
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
		showcase.HeartBoardMessageID = 1
		if msgErr != nil {
			logger.Error(msgErr)
		}
	}

	err = common.GORM.Model(showcase).Update([]interface{}{showcase}).Error
	if err != nil {
		logger.Error(err)
	}
}

func generateShowcaseEmbed(showcase *Showcase) *discordgo.MessageEmbed {
	member, err := bot.GetMember(showcase.GuildID, showcase.AuthorID)
	if err != nil {
		logger.Error("memberErr: ", showcase.GuildID, showcase.AuthorID, err)
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
			Name:    bot.GetName(member),
			IconURL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("❤️ %d | Created by: %s", showcase.Approval, member.User.Username),
		},
	}

	return embed
}

var audioRegex = regexp.MustCompile(`ogg|mp3`)
var videoRegex = regexp.MustCompile(`mp4|mov|wav|webm`)

func generateMemberQuoteEmbed(memberQuote *MemberQuote, quoteEmoji string) *discordgo.MessageEmbed {
	member, err := bot.GetMember(memberQuote.GuildID, memberQuote.AuthorID)
	messageLink := fmt.Sprintf("https://discord.com/channels/%d/%d/%d", memberQuote.GuildID, memberQuote.ChannelID, memberQuote.MessageID)

	if err != nil {
		logger.Error("memberErr: ", memberQuote.GuildID, memberQuote.AuthorID, err)
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Color: rand.Intn(16777215),
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Author", Value: member.User.Mention(), Inline: true},
			{Name: "Channel", Value: fmt.Sprintf("<#%d>", memberQuote.ChannelID), Inline: true},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("%s %d | %d", quoteEmoji, memberQuote.Approval, memberQuote.MessageID),
		},
		Timestamp: memberQuote.QuoteTimestamp.Format(time.RFC3339),
	}

	if memberQuote.Content != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Content", Value: memberQuote.Content})
	}
	if audioRegex.MatchString(memberQuote.ImageUrl) {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "File Type", Value: "Audio"})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Message", Value: fmt.Sprintf("[Jump to](%s)", messageLink)})
	} else if videoRegex.MatchString(memberQuote.ImageUrl) {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "File Type", Value: "Video"})
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Message", Value: fmt.Sprintf("[Jump to](%s)", messageLink)})
	} else {
		embed.Image = &discordgo.MessageEmbedImage{
			URL: memberQuote.ImageUrl,
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Message", Value: fmt.Sprintf("[Jump to](%s)", messageLink)})
	}

	return embed
}

func handleCountApproval(reactions []*discordgo.MessageReactions, EmojiName string) int64 {
	count := 0

	for _, reaction := range reactions {
		if reaction.Emoji.Name == EmojiName {
			count += reaction.Count
		}

		if reaction.Emoji.Name == "❌" {
			count -= reaction.Count
		}
	}

	return int64(count)
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
