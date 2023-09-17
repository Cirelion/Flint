package moderation

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
)

type ModlogAction struct {
	Prefix string
	Emoji  string
	Color  int

	Footer string
}

func (m ModlogAction) String() string {
	str := m.Emoji + m.Prefix
	if m.Footer != "" {
		str += " (" + m.Footer + ")"
	}

	return str
}

var (
	MAMute           = ModlogAction{Prefix: "Muted", Emoji: "🔇", Color: 0x57728e}
	MAUnmute         = ModlogAction{Prefix: "Unmuted", Emoji: "🔊", Color: 0x62c65f}
	MAKick           = ModlogAction{Prefix: "Kicked", Emoji: "👢", Color: 0xf2a013}
	MABanned         = ModlogAction{Prefix: "Banned", Emoji: "🔨", Color: 0xd64848}
	MAUnbanned       = ModlogAction{Prefix: "Unbanned", Emoji: "🔓", Color: 0x62c65f}
	MAWarned         = ModlogAction{Prefix: "Warned", Emoji: "⚠", Color: 0xfca253}
	MATimeoutAdded   = ModlogAction{Prefix: "Timed out", Emoji: "⏱", Color: 0x9b59b6}
	MATimeoutRemoved = ModlogAction{Prefix: "Timeout removed from", Emoji: "⏱", Color: 0x9b59b6}
	MAGiveRole       = ModlogAction{Prefix: "", Emoji: "➕", Color: 0x53fcf9}
	MARemoveRole     = ModlogAction{Prefix: "", Emoji: "➖", Color: 0x53fcf9}
	MAClearWarnings  = ModlogAction{Prefix: "Cleared warnings", Emoji: "👌", Color: 0x62c65f}
)

func generateGenericModEmbed(action ModlogAction, author *discordgo.User, target *discordgo.User, reason string, duration time.Duration, logLink string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: fmt.Sprintf("User %s", action.Prefix),
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: discordgo.EndpointUserAvatar(target.ID, target.Avatar),
		},
		Color:       action.Color,
		Description: fmt.Sprintf(">>> **User:** %s (%d)\n**Reason:** %s", target.Mention(), target.ID, reason),
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if duration > 0 {
		embed.Description = embed.Description + fmt.Sprintf("\n**Duration:** %s", common.HumanizeDuration(common.DurationPrecisionMinutes, duration))
	}

	if logLink != "" {
		embed.URL = logLink
	}

	if !author.Bot {
		embed.Footer = &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Action taken by %s", author.Globalname),
			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
		}
	}

	return embed
}

func CreateModlogEmbed(config *Config, author *discordgo.User, action ModlogAction, target *discordgo.User, reason, logLink string, duration time.Duration) error {
	channelID := config.IntActionChannel()
	config.GetGuildID()
	if channelID == 0 {
		return nil
	}
	embed := generateGenericModEmbed(action, author, target, reason, duration, logLink)
	_, err := common.BotSession.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		if common.IsDiscordErr(err, discordgo.ErrCodeMissingAccess, discordgo.ErrCodeMissingPermissions, discordgo.ErrCodeUnknownChannel) {
			// disable the modlog
			config.ActionChannel = ""
			config.Save(config.GetGuildID())
			return nil
		}
		return err
	}

	return err
}

var (
	logsRegex = regexp.MustCompile(`\(\[Logs\]\(.*\)\)`)
)

func updateEmbedReason(author *discordgo.User, reason string, embed *discordgo.MessageEmbed) {
	const checkStr = "📄**Reason:**"

	index := strings.Index(embed.Description, checkStr)
	withoutReason := embed.Description[:index+len(checkStr)]

	logsLink := logsRegex.FindString(embed.Description)
	if logsLink != "" {
		logsLink = " " + logsLink
	}

	embed.Description = withoutReason + " " + reason + logsLink

	if author != nil {
		embed.Author = &discordgo.MessageEmbedAuthor{
			Name:    fmt.Sprintf("%s (ID %d)", author.String(), author.ID),
			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
		}
	}
}
