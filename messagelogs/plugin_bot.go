package messagelogs

import (
	"bytes"
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/bot/eventsystem"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	"github.com/cirelion/flint/moderation"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLast(p, HandleMsgCreate, eventsystem.EventMessageCreate)
	eventsystem.AddHandlerAsyncLast(p, HandleMsgUpdate, eventsystem.EventMessageUpdate)
	eventsystem.AddHandlerAsyncLast(p, HandleMsgDelete, eventsystem.EventMessageDelete, eventsystem.EventMessageDeleteBulk)
}

func HandleMsgCreate(evt *eventsystem.EventData) (retry bool, err error) {
	if evt.GS == nil {
		return false, nil
	}
	config, err := moderation.GetConfig(evt.GS.ID)
	if err != nil {
		return false, err
	}
	msg := evt.MessageCreate()
	channel := evt.GS.GetChannelOrThread(msg.ChannelID)
	re, _ := regexp.Compile("^(?:[^<]|\\A)https://(?:\\w+\\.)?discord(?:app)?\\.com/channels\\/(\\d+)\\/(\\d+)\\/(\\d+)(?:[^>\\d]|\\z)$")

	if IsIgnoredChannel(config, channel.ID, channel.ParentID) || msg.Author.Bot || re.MatchString(msg.Content) {
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
		var attachments []Attachment
		for _, attachment := range msg.Attachments {
			attachments = append(attachments, Attachment{
				ID:        attachment.ID,
				MessageID: msg.ID,
				Filename:  attachment.Filename,
				Url:       attachment.URL,
				ProxyUrl:  attachment.ProxyURL,
			})
		}

		message.Attachments = attachments
	}
	err = common.GORM.Model(&message).Save(&message).Error

	return false, err
}

func HandleMsgUpdate(evt *eventsystem.EventData) (retry bool, err error) {
	if evt.GS == nil {
		return false, nil
	}

	config, err := moderation.GetConfig(evt.GS.ID)
	msg := evt.MessageUpdate()
	if msg.Interaction != nil {
		return false, nil
	}

	channelCategory := evt.GS.GetChannelOrThread(msg.ChannelID).ParentID
	logChannel := config.IntEditLogChannel()

	if IsIgnoredChannel(config, msg.ChannelID, channelCategory) {
		return false, nil
	}

	if IsModChannel(config, msg.ChannelID, channelCategory) {
		logChannel = config.IntModLogChannel()
	}

	message := &Message{MessageID: msg.ID}
	err = common.GORM.Model(&message).Preload("Attachments").First(&message).Error
	if err != nil || message.Content == msg.Content {
		return false, nil
	}

	originalContent := message.OriginalContent
	if originalContent == "" {
		originalContent = "No content"
	}
	message.Content = msg.Content
	message.OriginalContent = msg.Content

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
				Value: originalContent,
			},
			{
				Name:  "After",
				Value: message.Content,
			},
		},
	}

	if originalContent == "No content" && len(message.Attachments) == 1 {
		embed.Image = &discordgo.MessageEmbedImage{
			URL:      message.Attachments[0].Url,
			ProxyURL: message.Attachments[0].Url,
		}
	}

	_, err = common.BotSession.ChannelMessageSendEmbed(logChannel, embed)

	return false, err
}

func HandleMsgDelete(evt *eventsystem.EventData) (retry bool, err error) {
	if evt.GS == nil {
		return false, nil
	}
	config, err := moderation.GetConfig(evt.GS.ID)
	logChannel := config.IntDeleteLogChannel()

	if evt.Type == eventsystem.EventMessageDelete {
		msg := evt.MessageDelete()
		if msg.ID == 0 {
			return
		}

		channelCategory := evt.GS.GetChannelOrThread(msg.ChannelID).ParentID
		if IsIgnoredChannel(config, msg.ChannelID, channelCategory) {
			return false, nil
		}

		if IsModChannel(config, msg.ChannelID, channelCategory) {
			logChannel = config.IntModLogChannel()
		}

		_, err = DeleteAndLogMessages(evt.Session, evt.GS.ID, logChannel, msg.ID)
	} else if evt.Type == eventsystem.EventMessageDeleteBulk {
		messageDeleteBulk := evt.MessageDeleteBulk()
		channelCategory := evt.GS.GetChannelOrThread(messageDeleteBulk.ChannelID).ParentID

		if IsIgnoredChannel(config, messageDeleteBulk.ChannelID, channelCategory) {
			return false, nil
		}

		if IsModChannel(config, messageDeleteBulk.ChannelID, channelCategory) {
			logChannel = config.IntModLogChannel()
		}

		for _, msg := range messageDeleteBulk.Messages {
			_, err = DeleteAndLogMessages(evt.Session, evt.GS.ID, logChannel, msg)
		}
	}

	return false, err
}

func DeleteAndLogMessages(session *discordgo.Session, guildID int64, deleteLogChannelID int64, messageID int64) (retry bool, err error) {
	message := &Message{MessageID: messageID}
	err = common.GORM.Model(&message).Preload("Attachments").First(&message).Error
	if message.AuthorID == 0 {
		return false, nil
	}

	member, _ := bot.GetMember(guildID, message.AuthorID)

	embed, err := GenerateDeleteEmbed(session, guildID, message, member)
	if err != nil {
		return false, err
	}
	messageSend := &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}}

	var fileName string
	if len(message.Attachments) == 1 {
		fileName = DownloadFile(message.Attachments[0].Url)
		file, fileErr := os.Open(fileName)
		if fileErr != nil {
			return false, fileErr
		}

		if strings.Contains(fileName, ".ogg") {
			_, err = common.BotSession.ChannelMessageSendComplex(deleteLogChannelID, &discordgo.MessageSend{Files: []*discordgo.File{{Name: fileName, Reader: file}}})
		} else {
			messageSend.Files = []*discordgo.File{{Name: fileName, Reader: file}}
		}
	}

	if len(message.Content) > 1024 {
		var buf bytes.Buffer
		buf.WriteString(message.Content)

		files := []*discordgo.File{
			{Name: "Message content.txt", ContentType: "text/plain", Reader: &buf},
		}

		messageSend.Files = append(messageSend.Files, files...)
	}

	_, err = common.BotSession.ChannelMessageSendComplex(deleteLogChannelID, messageSend)
	os.Remove(fileName)
	if err != nil {
		return false, err
	}

	if len(message.Attachments) != 1 {
		for _, attachment := range message.Attachments {
			fileName = DownloadFile(attachment.Url)
			file, fileErr := os.Open(fileName)
			if fileErr != nil {
				return false, fileErr
			}

			_, err = common.BotSession.ChannelMessageSendComplex(deleteLogChannelID, &discordgo.MessageSend{Files: []*discordgo.File{{Name: fileName, Reader: file}}})
			os.Remove(fileName)
		}
	}

	return false, err
}

func IsIgnoredChannel(config *moderation.Config, channelID int64, channelCategory int64) bool {
	if channelCategory != 0 {
		if common.ContainsInt64Slice(config.IgnoreCategories, channelCategory) {
			return true
		}
	}

	return common.ContainsInt64Slice(config.IgnoreChannels, channelID)
}

func IsModChannel(config *moderation.Config, channelID int64, channelCategory int64) bool {
	if channelCategory != 0 {
		if common.ContainsInt64Slice(config.ModChannels, channelCategory) {
			return true
		}
	}

	return common.ContainsInt64Slice(config.ModChannels, channelID)
}

func GenerateDeleteEmbed(session *discordgo.Session, guildID int64, message *Message, member *dstate.MemberState) (*discordgo.MessageEmbed, error) {
	embed := &discordgo.MessageEmbed{
		Color:       0xd64848,
		Description: fmt.Sprintf("Message from <@%d> deleted in <#%d>.\nIt was sent on <t:%d:f>", message.AuthorID, message.ChannelID, message.CreatedAt.Unix()),
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("User ID: %d", message.AuthorID),
		},
	}

	if member != nil {
		embed.Author = &discordgo.MessageEmbedAuthor{
			Name:    member.User.Username,
			IconURL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		}
	}

	if message.MessageReferenceID != 0 {
		messageReference, err := session.ChannelMessage(message.MessageReferenceChannelID, message.MessageReferenceID)
		if err != nil {
			return nil, err
		}
		embed.Description += fmt.Sprintf("\n [Replied to @%s](https://discord.com/channels/%d/%d/%d): %s", messageReference.Author.Username, guildID, messageReference.ChannelID, messageReference.ID, messageReference.Content)
	}

	if message.Content != "" {
		if len(message.Content) > 1024 {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "Message Content",
				Value: "Content exceeds 1024 in length, see attached file.",
			})
		} else {
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  "Message Content",
				Value: message.Content,
			})
		}
	}

	if len(message.Attachments) == 1 {
		fileName := message.Attachments[0].Filename
		if strings.Contains(fileName, "mp4") {
			embed.Video = &discordgo.MessageEmbedVideo{
				URL: fmt.Sprintf("attachment://%s", message.Attachments[0].Filename),
			}
		} else {
			embed.Image = &discordgo.MessageEmbedImage{
				URL: fmt.Sprintf("attachment://%s", message.Attachments[0].Filename),
			}
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

func DownloadFile(filePath string) string {
	fileURL, err := url.Parse(filePath)
	if err != nil {
		log.Fatal(err)
	}

	path := fileURL.Path
	segments := strings.Split(path, "/")
	fileName := segments[len(segments)-1]

	file, err := os.Create(fileName)
	if err != nil {
		log.Fatal(err)
	}

	client := http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			r.URL.Opaque = r.URL.Path
			return nil
		},
	}

	resp, err := client.Get(filePath)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	return file.Name()
}
