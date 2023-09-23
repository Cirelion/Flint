package messagelogs

import (
	"fmt"
	"github.com/botlabs-gg/yagpdb/v2/bot"
	"github.com/botlabs-gg/yagpdb/v2/bot/eventsystem"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
	"github.com/botlabs-gg/yagpdb/v2/lib/dstate"
	"github.com/botlabs-gg/yagpdb/v2/moderation"
	"regexp"
	"time"
)

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLast(p, HandleMsgCreate, eventsystem.EventMessageCreate)
	eventsystem.AddHandlerAsyncLast(p, HandleMsgUpdate, eventsystem.EventMessageUpdate)
	eventsystem.AddHandlerAsyncLast(p, HandleMsgDelete, eventsystem.EventMessageDelete, eventsystem.EventMessageDeleteBulk)
}

func HandleMsgCreate(evt *eventsystem.EventData) (retry bool, err error) {
	config, err := moderation.GetConfig(evt.GS.ID)
	if err != nil {
		return false, err
	}

	msg := evt.MessageCreate()
	channelCategory := evt.GS.GetChannel(msg.ChannelID).ParentID

	if IsIgnoredChannel(config, channelCategory, msg.ChannelID) {
		return false, nil
	}
	message := &Message{
		MessageID:       msg.ID,
		ChannelID:       msg.ChannelID,
		AuthorID:        msg.Author.ID,
		Content:         msg.Content,
		OriginalContent: msg.Content,
	}

	if len(msg.StickerItems) > 0 {
		message.StickerID = msg.StickerItems[0].ID
		message.StickerName = msg.StickerItems[0].Name
		message.StickerFormatType = msg.StickerItems[0].FormatType
	}

	if msg.MessageReference != nil {
		message.MessageReferenceID = msg.MessageReference.MessageID
		message.MessageReferenceChannelID = msg.MessageReference.ChannelID
	}

	if len(msg.Attachments) > 0 {
		message.AttachmentUrl = msg.Attachments[0].URL
	}

	err = common.GORM.Model(&message).Save(&message).Error

	return false, err
}

func HandleMsgUpdate(evt *eventsystem.EventData) (retry bool, err error) {
	config, err := moderation.GetConfig(evt.GS.ID)
	msg := evt.MessageUpdate()
	if msg.Interaction != nil {
		return false, nil
	}

	channelCategory := evt.GS.GetChannel(msg.ChannelID).ParentID
	if IsIgnoredChannel(config, channelCategory, msg.ChannelID) {
		return false, nil
	}

	message := &Message{MessageID: msg.ID}
	err = common.GORM.Model(&message).First(&message).Error

	if err != nil {
		return false, err
	}

	message.Content = msg.Content

	err = common.GORM.Model(&message).Update(&message).Error
	if err != nil {
		return false, err
	}

	member, err := bot.GetMember(evt.GS.ID, message.AuthorID)
	if err != nil {
		return false, err
	}

	messageUrl := fmt.Sprintf("https://discord.com/channels/%d/%d/%d", evt.GS.ID, message.ChannelID, message.MessageID)

	embed := &discordgo.MessageEmbed{
		Color: 0xf2a013,
		Author: &discordgo.MessageEmbedAuthor{
			Name:    member.User.Username,
			IconURL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		},
		Description: fmt.Sprintf("Message from <@%d> edited in <#%d>.\n[Jump to message](%s)", message.AuthorID, message.ChannelID, messageUrl),
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("User ID: %d", message.AuthorID),
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "Before",
				Value: message.OriginalContent,
			},
			{
				Name:  "After",
				Value: message.Content,
			},
		},
	}

	_, err = common.BotSession.ChannelMessageSendEmbed(config.IntEditLogChannel(), embed)

	return false, err
}

func HandleMsgDelete(evt *eventsystem.EventData) (retry bool, err error) {
	config, err := moderation.GetConfig(evt.GS.ID)
	if evt.Type == eventsystem.EventMessageDelete {
		msg := evt.MessageDelete()
		channelCategory := evt.GS.GetChannel(msg.ChannelID).ParentID

		if IsIgnoredChannel(config, channelCategory, msg.ChannelID) {
			return false, nil
		}

		_, err = DeleteAndLogMessages(evt.Session, evt.GS.ID, config.IntDeleteLogChannel(), msg.ID)
	} else if evt.Type == eventsystem.EventMessageDeleteBulk {
		messageDeleteBulk := evt.MessageDeleteBulk()
		channelCategory := evt.GS.GetChannel(messageDeleteBulk.ChannelID).ParentID

		if IsIgnoredChannel(config, channelCategory, messageDeleteBulk.ChannelID) {
			return false, nil
		}

		for _, msg := range messageDeleteBulk.Messages {
			_, err = DeleteAndLogMessages(evt.Session, evt.GS.ID, config.IntDeleteLogChannel(), int64(msg))
		}
	}

	return false, err
}

func DeleteAndLogMessages(session *discordgo.Session, guildID int64, deleteLogChannelID int64, messageID int64) (retry bool, err error) {
	message := &Message{MessageID: messageID}

	err = common.GORM.Model(&message).First(&message).Error

	member, err := bot.GetMember(guildID, message.AuthorID)
	if err != nil {
		return false, err
	}

	embed, err := GenerateDeleteEmbed(session, guildID, message, member)
	if err != nil {
		return false, err
	}

	_, err = common.BotSession.ChannelMessageSendEmbed(deleteLogChannelID, embed)
	if err != nil {
		return false, err
	}

	regexResult, _ := regexp.MatchString("(mp4|avi|wmv|mov|flv|mkv|webm|vob|ogv|m4v|3gp|3g2|mpeg|mpg|m2v|svi|3gpp|3gpp2|mxf|roq|nsv|f4v|f4p|f4a|f4b)", message.AttachmentUrl)
	if regexResult {
		_, err = common.BotSession.ChannelMessageSend(deleteLogChannelID, message.AttachmentUrl)
	}

	return false, err
}

func IsIgnoredChannel(config *moderation.Config, channelCategory int64, channelID int64) bool {
	if channelCategory != 0 {
		if common.ContainsInt64Slice(config.IgnoreCategories, channelCategory) {
			return true
		}
	}

	return common.ContainsInt64Slice(config.IgnoreChannels, channelID)
}

func GenerateDeleteEmbed(session *discordgo.Session, guildID int64, message *Message, member *dstate.MemberState) (*discordgo.MessageEmbed, error) {
	embed := &discordgo.MessageEmbed{
		Color: 0xd64848,
		Author: &discordgo.MessageEmbedAuthor{
			Name:    member.User.Username,
			IconURL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		},
		Description: fmt.Sprintf("Message from <@%d> deleted in <#%d>.\nIt was sent on <t:%d:f>", message.AuthorID, message.ChannelID, message.CreatedAt.Unix()),
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("User ID: %d", message.AuthorID),
		},
	}

	if message.MessageReferenceID != 0 {
		messageReference, err := session.ChannelMessage(message.MessageReferenceChannelID, message.MessageReferenceID)
		if err != nil {
			return nil, err
		}
		embed.Description += fmt.Sprintf("\n [Replied to @%s](https://discord.com/channels/%d/%d/%d): %s", messageReference.Author.Username, guildID, messageReference.ChannelID, messageReference.ID, messageReference.Content)
	}

	if message.Content != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  "Message Content",
			Value: message.Content,
		})
	}

	if message.AttachmentUrl != "" {
		embed.Image = &discordgo.MessageEmbedImage{
			URL: message.AttachmentUrl,
		}
	} else if message.StickerID != 0 {
		if message.StickerFormatType == discordgo.StickerLOTTIE {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "Discord Default Sticker",
				Value: message.StickerName,
			})
		} else {
			format := "png"
			if message.StickerFormatType == discordgo.StickerGIF {
				format = "gif"
			}

			embed.Image = &discordgo.MessageEmbedImage{
				URL: fmt.Sprintf("https://media.discordapp.net/stickers/%d.%s", message.StickerID, format),
			}
		}
	}

	return embed, nil
}
