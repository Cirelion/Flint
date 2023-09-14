package moderation

import (
	"bytes"
	"context"
	"database/sql"
	"emperror.dev/errors"
	"fmt"
	"github.com/botlabs-gg/yagpdb/v2/analytics"
	"github.com/botlabs-gg/yagpdb/v2/bot"
	"github.com/botlabs-gg/yagpdb/v2/bot/paginatedmessages"
	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/common/scheduledevents2"
	"github.com/botlabs-gg/yagpdb/v2/common/templates"
	"github.com/botlabs-gg/yagpdb/v2/customcommands/models"
	"github.com/botlabs-gg/yagpdb/v2/lib/dcmd"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
	"github.com/botlabs-gg/yagpdb/v2/lib/dstate"
	"github.com/botlabs-gg/yagpdb/v2/logs"
	"github.com/botlabs-gg/yagpdb/v2/web"
	"github.com/jinzhu/gorm"
	log "github.com/sirupsen/logrus"
	"github.com/vmihailenco/msgpack"
	"github.com/volatiletech/sqlboiler/queries/qm"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type LightDBEntry struct {
	ID      int64
	GuildID int64
	UserID  int64

	CreatedAt time.Time
	UpdatedAt time.Time

	Key   string
	Value interface{}

	User discordgo.User

	ExpiresAt time.Time
}

func MBaseCmd(cmdData *dcmd.Data, targetID int64) (config *Config, targetUser *discordgo.User, err error) {
	config, err = GetConfig(cmdData.GuildData.GS.ID)
	if err != nil {
		return nil, nil, errors.WithMessage(err, "GetConfig")
	}

	if targetID != 0 {
		targetMember, _ := bot.GetMember(cmdData.GuildData.GS.ID, targetID)
		if targetMember != nil {
			gs := cmdData.GuildData.GS

			above := bot.IsMemberAbove(gs, cmdData.GuildData.MS, targetMember)

			if !above {
				return config, &targetMember.User, commands.NewUserError("Can't use moderation commands on users ranked the same or higher than you")
			}

			return config, &targetMember.User, nil
		}
	}

	return config, &discordgo.User{
		Username:      "unknown",
		Discriminator: "????",
		ID:            targetID,
	}, nil

}

func MBaseCmdSecond(cmdData *dcmd.Data, reason string, reasonArgOptional bool, neededPerm int64, additionalPermRoles []int64, enabled bool) (oreason string, err error) {
	cmdName := cmdData.Cmd.Trigger.Names[0]
	oreason = reason
	if !enabled {
		return oreason, commands.NewUserErrorf("The **%s** command is disabled on this server. It can be enabled at <%s/moderation>", cmdName, web.ManageServerURL(cmdData.GuildData))
	}

	if strings.TrimSpace(reason) == "" {
		if !reasonArgOptional {
			return oreason, commands.NewUserError("A reason has been set to be required for this command by the server admins, see help for more info.")
		}

		oreason = "(No reason specified)"
	}

	member := cmdData.GuildData.MS

	// check permissions or role setup for this command
	permsMet := false
	if len(additionalPermRoles) > 0 {
		// Check if the user has one of the required roles
		for _, r := range member.Member.Roles {
			if common.ContainsInt64Slice(additionalPermRoles, r) {
				permsMet = true
				break
			}
		}
	}

	if !permsMet && neededPerm != 0 {
		// Fallback to legacy permissions
		hasPerms, err := bot.AdminOrPermMS(cmdData.GuildData.GS.ID, cmdData.ChannelID, member, neededPerm)
		if err != nil || !hasPerms {
			return oreason, commands.NewUserErrorf("The **%s** command requires the **%s** permission in this channel or additional roles set up by admins, you don't have it. (if you do contact bot support)", cmdName, common.StringPerms[neededPerm])
		}

		permsMet = true
	}

	go analytics.RecordActiveUnit(cmdData.GuildData.GS.ID, &Plugin{}, "executed_cmd_"+cmdName)

	return oreason, nil
}

func checkHierarchy(cmdData *dcmd.Data, targetID int64) error {
	botMember, err := bot.GetMember(cmdData.GuildData.GS.ID, common.BotUser.ID)
	if err != nil {
		return commands.NewUserError("Failed fetching bot member to check hierarchy")
	}

	gs := cmdData.GuildData.GS
	targetMember, err := bot.GetMember(gs.ID, targetID)
	if err != nil {
		return nil
	}

	above := bot.IsMemberAbove(gs, botMember, targetMember)

	if !above {
		cmdName := cmdData.Cmd.Trigger.Names[0]
		return commands.NewUserErrorf("Can't use the **%s** command on members that are ranked higher than the bot.", cmdName)
	}

	return nil
}

func SafeArgString(data *dcmd.Data, arg int) string {
	if arg >= len(data.Args) || data.Args[arg].Value == nil {
		return ""
	}

	return data.Args[arg].Str()
}

func GenericCmdResp(action ModlogAction, target *discordgo.User, duration time.Duration, zeroDurPermanent bool, noDur bool) string {
	durStr := " indefinitely"
	if duration > 0 || !zeroDurPermanent {
		durStr = " for `" + common.HumanizeDuration(common.DurationPrecisionMinutes, duration) + "`"
	}
	if noDur {
		durStr = ""
	}

	userStr := target.String()
	if target.Discriminator == "????" {
		userStr = strconv.FormatInt(target.ID, 10)
	}

	return fmt.Sprintf("%s %s `%s`%s", action.Emoji, action.Prefix, userStr, durStr)
}

var ModerationCommands = []*commands.YAGCommand{
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Ban",
		Aliases:       []string{"banid"},
		Description:   "Bans a member, specify number of days of messages to delete with -ddays (0 to 7)",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Duration", Type: &commands.DurationArg{}, Default: time.Duration(0)},
			{Name: "Reason", Type: dcmd.String},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "ddays", Help: "Number of days of messages to delete", Type: dcmd.Int},
		},
		RequiredDiscordPermsHelp: "BanMembers or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionBanMembers}},
		ArgumentCombos:           [][]int{{0, 1, 2}, {0, 2, 1}, {0, 1}, {0, 2}, {0}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			reason := SafeArgString(parsed, 2)
			reason, err = MBaseCmdSecond(parsed, reason, config.BanReasonOptional, discordgo.PermissionBanMembers, config.BanCmdRoles, config.BanEnabled)
			if err != nil {
				return nil, err
			}

			if utf8.RuneCountInString(reason) > 470 {
				return "Error: Reason too long (can be max 470 characters).", nil
			}

			err = checkHierarchy(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			ddays := int(config.DefaultBanDeleteDays.Int64)
			if parsed.Switches["ddays"].Value != nil {
				ddays = parsed.Switches["ddays"].Int()
			}
			banDuration := parsed.Args[1].Value.(time.Duration)

			var msg *discordgo.Message
			if parsed.TraditionalTriggerData != nil {
				msg = parsed.TraditionalTriggerData.Message
			}
			err = BanUserWithDuration(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, target, banDuration, ddays)
			if err != nil {
				return nil, err
			}

			return GenericCmdResp(MABanned, target, banDuration, true, false), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Unban",
		Aliases:       []string{"unbanid"},
		Description:   "Unbans a user. Reason requirement is same as ban command setting.",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "BanMembers or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionBanMembers}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _, err := MBaseCmd(parsed, 0) //No need to check member role hierarchy as banned members should not be in server
			if err != nil {
				return nil, err
			}

			reason := SafeArgString(parsed, 1)
			reason, err = MBaseCmdSecond(parsed, reason, config.BanReasonOptional, discordgo.PermissionBanMembers, config.BanCmdRoles, config.BanEnabled)
			if err != nil {
				return nil, err
			}
			targetID := parsed.Args[0].Int64()
			target := &discordgo.User{
				Username:      "unknown",
				Discriminator: "????",
				ID:            targetID,
			}
			targetMem, _ := bot.GetMember(parsed.GuildData.GS.ID, targetID)
			if targetMem != nil {
				return "User is not banned!", nil
			}

			isNotBanned, err := UnbanUser(config, parsed.GuildData.GS.ID, parsed.Author, reason, target)

			if err != nil {
				return nil, err
			}
			if isNotBanned {
				return "User is not banned!", nil
			}

			return GenericCmdResp(MAUnbanned, target, 0, true, true), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Kick",
		Description:   "Kicks a member",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "KickMembers or ManageServer",
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "cl", Help: "Messages to delete", Type: &dcmd.IntArg{Min: 1, Max: 100}},
		},
		RequireBotPerms:     [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionKickMembers}},
		SlashCommandEnabled: true,
		IsResponseEphemeral: true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			reason := SafeArgString(parsed, 1)
			reason, err = MBaseCmdSecond(parsed, reason, config.KickReasonOptional, discordgo.PermissionKickMembers, config.KickCmdRoles, config.KickEnabled)
			if err != nil {
				return nil, err
			}

			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			if utf8.RuneCountInString(reason) > 470 {
				return "Error: Reason too long (can be max 470 characters).", nil
			}

			err = checkHierarchy(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			toDel := -1
			if parsed.Switches["cl"].Value != nil {
				toDel = parsed.Switches["cl"].Int()
			}

			var msg *discordgo.Message
			if parsed.TraditionalTriggerData != nil {
				msg = parsed.TraditionalTriggerData.Message
			}

			err = KickUser(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, target, toDel)
			if err != nil {
				return nil, err
			}

			return GenericCmdResp(MAKick, target, 0, true, true), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Mute",
		Description:   "Mutes a member",
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Duration", Type: &commands.DurationArg{}},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "KickMembers or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionManageRoles}},
		ArgumentCombos:           [][]int{{0, 1, 2}, {0, 2, 1}, {0, 1}, {0, 2}, {0}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			if config.MuteRole == "" {
				return fmt.Sprintf("No mute role selected. Select one at <%s/moderation>", web.ManageServerURL(parsed.GuildData)), nil
			}

			reason := parsed.Args[2].Str()
			reason, err = MBaseCmdSecond(parsed, reason, config.MuteReasonOptional, discordgo.PermissionKickMembers, config.MuteCmdRoles, config.MuteEnabled)
			if err != nil {
				return nil, err
			}

			d := time.Duration(config.DefaultMuteDuration.Int64) * time.Minute
			if parsed.Args[1].Value != nil {
				d = parsed.Args[1].Value.(time.Duration)
			}
			if d > 0 && d < time.Minute {
				d = time.Minute
			}

			logger.Info(d.Seconds())

			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			var msg *discordgo.Message
			if parsed.TraditionalTriggerData != nil {
				msg = parsed.TraditionalTriggerData.Message
			}
			err = MuteUnmuteUser(config, true, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, member, int(d.Minutes()))
			if err != nil {
				return nil, err
			}

			common.BotSession.GuildMemberMove(parsed.GuildData.GS.ID, target.ID, 0)
			return GenericCmdResp(MAMute, target, d, true, false), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Unmute",
		Description:   "Unmutes a member",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "KickMembers or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionManageRoles}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			if config.MuteRole == "" {
				return "No mute role set up, assign a mute role in the control panel", nil
			}

			reason := parsed.Args[1].Str()
			reason, err = MBaseCmdSecond(parsed, reason, config.UnmuteReasonOptional, discordgo.PermissionKickMembers, config.MuteCmdRoles, config.MuteEnabled)
			if err != nil {
				return nil, err
			}

			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			var msg *discordgo.Message
			if parsed.TraditionalTriggerData != nil {
				msg = parsed.TraditionalTriggerData.Message
			}
			err = MuteUnmuteUser(config, false, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, member, 0)
			if err != nil {
				return nil, err
			}

			return GenericCmdResp(MAUnmute, target, 0, false, true), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Timeout",
		Description:   "Timeout a member",
		Aliases:       []string{"to"},
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Duration", Type: &commands.DurationArg{}},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "TimeoutMembers/ModerateMembers or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionModerateMembers}},
		ArgumentCombos:           [][]int{{0, 1, 2}, {0, 2, 1}, {0, 1}, {0, 2}, {0}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			reason := parsed.Args[2].Str()
			reason, err = MBaseCmdSecond(parsed, reason, config.TimeoutReasonOptional, discordgo.PermissionModerateMembers, config.TimeoutCmdRoles, config.TimeoutEnabled)
			if err != nil {
				return nil, err
			}

			d := time.Duration(config.DefaultTimeoutDuration.Int64) * time.Minute
			if parsed.Args[1].Value != nil {
				d = parsed.Args[1].Value.(time.Duration)
			}
			if d < time.Minute {
				d = time.Minute
			}
			if d > MaxTimeOutDuration {
				return fmt.Sprintf("Error: Max duration of Timeouts can be %v days", (MaxTimeOutDuration.Hours() / 24)), nil
			}
			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			var msg *discordgo.Message
			if parsed.TraditionalTriggerData != nil {
				msg = parsed.TraditionalTriggerData.Message
			}
			err = TimeoutUser(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, &member.User, d)
			if err != nil {
				return nil, err
			}

			return GenericCmdResp(MATimeoutAdded, target, d, true, false), nil
		},
	}, {
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "RemoveTimeout",
		Aliases:       []string{"untimeout", "cleartimeout", "deltimeout", "rto"},
		Description:   "Removes a member's timeout",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "TimeoutMember/ModerateMember or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionModerateMembers}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			reason := parsed.Args[1].Str()
			reason, err = MBaseCmdSecond(parsed, reason, config.TimeoutReasonOptional, discordgo.PermissionModerateMembers, config.TimeoutCmdRoles, config.TimeoutEnabled)
			if err != nil {
				return nil, err
			}

			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			memberTimeout := member.Member.CommunicationDisabledUntil
			if memberTimeout == nil || memberTimeout.Before(time.Now()) {
				return "Member is not timed out", nil
			}

			err = RemoveTimeout(config, parsed.GuildData.GS.ID, parsed.Author, reason, &member.User)
			if err != nil {
				return nil, err
			}

			return GenericCmdResp(MATimeoutRemoved, target, 0, false, true), nil
		},
	},
	{
		CustomEnabled: true,
		Cooldown:      5,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Report",
		Description:   "Reports a member to the server's staff",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		SlashCommandEnabled: true,
		DefaultEnabled:      false,
		IsResponseEphemeral: true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _, err := MBaseCmd(parsed, 0)
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, 0, nil, config.ReportEnabled)
			if err != nil {
				return nil, err
			}

			temp, err := bot.GetMember(parsed.GuildData.GS.ID, parsed.Args[0].Int64())
			if err != nil || temp == nil {
				return nil, err
			}

			target := temp.User

			if target.ID == parsed.Author.ID {
				return "You can't report yourself, silly.", nil
			}

			logLink := CreateLogs(parsed.GuildData.GS.ID, parsed.GuildData.CS.ID, parsed.Author)

			channelID := config.IntReportChannel()
			if channelID == 0 {
				return "No report channel set up", nil
			}

			topContent := fmt.Sprintf("%s reported **%s (ID %d)**", parsed.Author.Mention(), target.String(), target.ID)

			embed := &discordgo.MessageEmbed{
				Author: &discordgo.MessageEmbedAuthor{
					Name:    fmt.Sprintf("%s (ID %d)", parsed.Author.String(), parsed.Author.ID),
					IconURL: discordgo.EndpointUserAvatar(parsed.Author.ID, parsed.Author.Avatar),
				},
				Description: fmt.Sprintf("🔍**Reported** %s *(ID %d)*\n📄**Reason:** %s ([Logs](%s))\n**Channel:** <#%d>", target.String(), target.ID, parsed.Args[1].Value, logLink, parsed.ChannelID),
				Color:       0xee82ee,
				Thumbnail: &discordgo.MessageEmbedThumbnail{
					URL: discordgo.EndpointUserAvatar(target.ID, target.Avatar),
				},
			}

			send := &discordgo.MessageSend{
				Content: topContent,
				Embeds:  []*discordgo.MessageEmbed{embed},
				AllowedMentions: discordgo.AllowedMentions{
					Parse: []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
				},
			}

			_, err = common.BotSession.ChannelMessageSendComplex(channelID, send)
			if err != nil {
				return "Something went wrong while sending your report!", err
			}

			// Don't bother sending confirmation if it is done in the report channel
			if channelID != parsed.ChannelID || parsed.SlashCommandTriggerData != nil {
				return "User reported to the proper authorities!", nil
			}

			return nil, nil
		},
	},
	{
		CustomEnabled:   true,
		CmdCategory:     commands.CategoryModeration,
		Name:            "Clean",
		Description:     "Delete the last number of messages from chat, optionally filtering by user, max age and regex or ignoring pinned messages.",
		LongDescription: "Specify a regex with \"-r regex_here\" and max age with \"-ma 1h10m\"\nYou can invert the regex match (i.e. only clear messages that do not match the given regex) by supplying the `-im` flag\nNote: Will only look in the last 1k messages",
		Aliases:         []string{"clear", "cl"},
		RequiredArgs:    1,
		Arguments: []*dcmd.ArgDef{
			{Name: "Num", Type: &dcmd.IntArg{Min: 1, Max: 100}},
			{Name: "User", Type: dcmd.UserID, Default: 0},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "r", Help: "Regex", Type: dcmd.String},
			{Name: "im", Help: "Invert regex match"},
			{Name: "ma", Help: "Max age", Default: time.Duration(0), Type: &commands.DurationArg{}},
			{Name: "minage", Help: "Min age", Default: time.Duration(0), Type: &commands.DurationArg{}},
			{Name: "i", Help: "Regex case insensitive"},
			{Name: "nopin", Help: "Ignore pinned messages"},
			{Name: "a", Help: "Only remove messages with attachments"},
			{Name: "to", Help: "Stop at this msg ID", Type: dcmd.BigInt},
			{Name: "from", Help: "Start at this msg ID", Type: dcmd.BigInt},
		},
		RequiredDiscordPermsHelp: "ManageMessages or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionManageMessages}},
		ArgumentCombos:           [][]int{{0}, {0, 1}, {1, 0}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _, err := MBaseCmd(parsed, 0)
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageMessages, nil, config.CleanEnabled)
			if err != nil {
				return nil, err
			}

			userFilter := parsed.Args[1].Int64()

			num := parsed.Args[0].Int()

			var triggerID int64
			ignoreTrigger := parsed.Source != dcmd.TriggerSourceDM && parsed.Context().Value(commands.CtxKeyExecutedByCC) == nil
			if ignoreTrigger {
				if parsed.TriggerType == dcmd.TriggerTypeSlashCommands {
					m, err := common.BotSession.GetOriginalInteractionResponse(common.BotApplication.ID, parsed.SlashCommandTriggerData.Interaction.Token)
					if err != nil {
						return nil, err
					}

					triggerID = m.ID
				} else {
					triggerID = parsed.TraditionalTriggerData.Message.ID
				}
			}

			if num > 100 {
				num = 100
			}

			if num < 1 {
				if num < 0 {
					return errors.New("Bot is having a stroke <https://www.youtube.com/watch?v=dQw4w9WgXcQ>"), nil
				}
				return errors.New("Can't delete nothing"), nil
			}

			filtered := false

			// Check if we should regex match this
			re := ""
			if parsed.Switches["r"].Value != nil {
				filtered = true
				re = parsed.Switches["r"].Str()

				// Add the case insensitive flag if needed
				if parsed.Switches["i"].Value != nil && parsed.Switches["i"].Value.(bool) {
					if !strings.HasPrefix(re, "(?i)") {
						re = "(?i)" + re
					}
				}
			}
			invertRegexMatch := parsed.Switch("im").Value != nil && parsed.Switch("im").Value.(bool)

			// Check if we have a max age
			ma := parsed.Switches["ma"].Value.(time.Duration)
			if ma != 0 {
				filtered = true
			}

			// Check if we have a min age
			minAge := parsed.Switches["minage"].Value.(time.Duration)
			if minAge != 0 {
				filtered = true
			}

			// Check if set to break at a certain ID
			toID := int64(0)
			if parsed.Switches["to"].Value != nil {
				filtered = true
				toID = parsed.Switches["to"].Int64()
			}

			// Check if set to break at a certain ID
			fromID := int64(0)
			if parsed.Switches["from"].Value != nil {
				filtered = true
				fromID = parsed.Switches["from"].Int64()
			}

			if toID > 0 && fromID > 0 && fromID < toID {
				return errors.New("from messageID cannot be less than to messageID"), nil
			}

			// Check if we should ignore pinned messages
			pe := false
			if parsed.Switches["nopin"].Value != nil && parsed.Switches["nopin"].Value.(bool) {
				pe = true
				filtered = true
			}

			// Check if we should only delete messages with attachments
			attachments := false
			if parsed.Switches["a"].Value != nil && parsed.Switches["a"].Value.(bool) {
				attachments = true
				filtered = true
			}

			limitFetch := num
			if userFilter != 0 || filtered {
				limitFetch = num * 50 // Maybe just change to full fetch?
			}

			if ignoreTrigger {
				limitFetch++
			}
			if limitFetch > 1000 {
				limitFetch = 1000
			}

			// Wait a second so the client dosen't gltich out
			time.Sleep(time.Second)

			numDeleted, err := AdvancedDeleteMessages(parsed.GuildData.GS.ID, parsed.ChannelID, triggerID, userFilter, re, invertRegexMatch, toID, fromID, ma, minAge, pe, attachments, num, limitFetch)
			deleteMessageWord := "messages"
			if numDeleted == 1 {
				deleteMessageWord = "message"
			}
			return dcmd.NewTemporaryResponse(time.Second*5, fmt.Sprintf("Deleted %d %s! :')", numDeleted, deleteMessageWord), true), err
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Reason",
		Description:   "Add/Edit a modlog reason",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "Message-ID", Type: dcmd.BigInt},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "KickMembers or ManageServer",
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _, err := MBaseCmd(parsed, 0)
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionKickMembers, nil, true)
			if err != nil {
				return nil, err
			}

			if config.ActionChannel == "" {
				return "No mod log channel set up", nil
			}

			msg, err := common.BotSession.ChannelMessage(config.IntActionChannel(), parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			if msg.Author.ID != common.BotUser.ID {
				return "I didn't make that message", nil
			}

			if len(msg.Embeds) < 1 {
				return "This entry is either too old or you're trying to mess with me...", nil
			}

			embed := msg.Embeds[0]
			updateEmbedReason(parsed.Author, parsed.Args[1].Str(), embed)
			_, err = common.BotSession.ChannelMessageEditEmbed(config.IntActionChannel(), msg.ID, embed)
			if err != nil {
				return nil, err
			}

			return "👌", nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Warn",
		Description:   "Warns a user, warnings are saved using the bot. Use -warnings to view them.",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "ManageMessages or ManageServer",
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}
			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageMessages, config.WarnCmdRoles, config.WarnCommandsEnabled)
			if err != nil {
				return nil, err
			}

			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			var msg *discordgo.Message
			if parsed.TraditionalTriggerData != nil {
				msg = parsed.TraditionalTriggerData.Message
			}
			err = WarnUser(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, target, parsed.Args[1].Str())
			if err != nil {
				return nil, err
			}

			return GenericCmdResp(MAWarned, target, 0, false, true), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Warnings",
		Description:   "Lists warning of a user.",
		Aliases:       []string{"Warns"},
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID, Default: 0},
			{Name: "Page", Type: &dcmd.IntArg{Max: 10000}, Default: 0},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "id", Help: "Warning ID", Type: dcmd.Int},
		},
		RequiredDiscordPermsHelp: "ManageMessages or ManageServer",
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			var err error
			config, _, err := MBaseCmd(parsed, 0)
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageMessages, config.WarnCmdRoles, true)
			if err != nil {
				return nil, err
			}

			if parsed.Switches["id"].Value != nil {
				var warn []*WarningModel
				err = common.GORM.Where("guild_id = ? AND id = ?", parsed.GuildData.GS.ID, parsed.Switches["id"].Int()).First(&warn).Error
				if err != nil && err != gorm.ErrRecordNotFound {
					return nil, err
				}
				if len(warn) == 0 {
					return fmt.Sprintf("Warning with given id : `%d` does not exist.", parsed.Switches["id"].Int()), nil
				}

				return &discordgo.MessageEmbed{
					Title:       fmt.Sprintf("Warning#%d - User : %s", warn[0].ID, warn[0].UserID),
					Description: fmt.Sprintf("<t:%d:f> - **Reason** : %s", warn[0].CreatedAt.Unix(), warn[0].Message),
					Footer:      &discordgo.MessageEmbedFooter{Text: fmt.Sprintf("By: %s (%13s)", warn[0].AuthorUsernameDiscrim, warn[0].AuthorID)},
				}, nil
			}
			page := parsed.Args[1].Int()
			if page < 1 {
				page = 1
			}
			if parsed.Context().Value(paginatedmessages.CtxKeyNoPagination) != nil {
				return PaginateWarnings(parsed)(nil, page)
			}
			_, err = paginatedmessages.CreatePaginatedMessage(parsed.GuildData.GS.ID, parsed.GuildData.CS.ID, page, 0, PaginateWarnings(parsed))
			return nil, err
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "EditWarning",
		Description:   "Edit a warning, id is the first number of each warning from the warnings command",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "Id", Type: dcmd.Int},
			{Name: "NewMessage", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "ManageMessages or ManageServer",
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _, err := MBaseCmd(parsed, 0)
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageMessages, config.WarnCmdRoles, config.WarnCommandsEnabled)
			if err != nil {
				return nil, err
			}

			rows := common.GORM.Model(WarningModel{}).Where("guild_id = ? AND id = ?", parsed.GuildData.GS.ID, parsed.Args[0].Int()).Update(
				"message", fmt.Sprintf("%s (updated by %s (%d))", parsed.Args[1].Str(), parsed.Author.String(), parsed.Author.ID)).RowsAffected

			if rows < 1 {
				return "Failed updating, most likely couldn't find the warning", nil
			}

			return "👌", nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "DelWarning",
		Aliases:       []string{"dw", "delwarn", "deletewarning"},
		Description:   "Deletes a warning, id is the first number of each warning from the warnings command",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "Id", Type: dcmd.Int},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "ManageMessages or ManageServer",
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _, err := MBaseCmd(parsed, 0)
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageMessages, config.WarnCmdRoles, config.WarnCommandsEnabled)
			if err != nil {
				return nil, err
			}

			rows := common.GORM.Where("guild_id = ? AND id = ?", parsed.GuildData.GS.ID, parsed.Args[0].Int()).Delete(WarningModel{}).RowsAffected
			if rows < 1 {
				return "Failed deleting, most likely couldn't find the warning", nil
			}

			return "👌", nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "ClearWarnings",
		Aliases:       []string{"clw"},
		Description:   "Clears the warnings of a user",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "ManageMessages or ManageServer",
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {

			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageMessages, config.WarnCmdRoles, config.WarnCommandsEnabled)
			if err != nil {
				return nil, err
			}

			rows := common.GORM.Where("guild_id = ? AND user_id = ?", parsed.GuildData.GS.ID, target.ID).Delete(WarningModel{}).RowsAffected

			reason := parsed.Args[1].Str()
			err = CreateModlogEmbed(config, parsed.Author, MAClearWarnings, target, reason, "")
			if err != nil {
				return "failed sending modlog", err
			}

			return fmt.Sprintf("Deleted %d warnings.", rows), nil
		},
	},
	{
		CmdCategory: commands.CategoryModeration,
		Name:        "TopWarnings",
		Aliases:     []string{"topwarns"},
		Description: "Shows ranked list of warnings on the server",
		Arguments: []*dcmd.ArgDef{
			{Name: "Page", Type: dcmd.Int, Default: 0},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "id", Help: "List userIDs"},
		},
		RequiredDiscordPermsHelp: "ManageMessages or ManageServer",
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: paginatedmessages.PaginatedCommand(0, func(parsed *dcmd.Data, p *paginatedmessages.PaginatedMessage, page int) (*discordgo.MessageEmbed, error) {

			showUserIDs := false
			config, _, err := MBaseCmd(parsed, 0)
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageMessages, config.WarnCmdRoles, true)
			if err != nil {
				return nil, err
			}

			if parsed.Switches["id"].Value != nil && parsed.Switches["id"].Value.(bool) {
				showUserIDs = true
			}

			offset := (page - 1) * 15
			entries, err := TopWarns(parsed.GuildData.GS.ID, offset, 15)
			if err != nil {
				return nil, err
			}

			if len(entries) < 1 && p != nil && p.LastResponse != nil { //Don't send No Results error on first execution.
				return nil, paginatedmessages.ErrNoResults
			}

			embed := &discordgo.MessageEmbed{
				Title: "Ranked list of warnings",
			}

			out := "```\n# - Warns - User\n"
			for _, v := range entries {
				if !showUserIDs {
					user := v.Username
					if user == "" {
						user = "unknown ID:" + strconv.FormatInt(v.UserID, 10)
					}
					out += fmt.Sprintf("#%02d: %4d - %s\n", v.Rank, v.WarnCount, user)
				} else {
					out += fmt.Sprintf("#%02d: %4d - %d\n", v.Rank, v.WarnCount, v.UserID)
				}
			}
			var count int
			common.GORM.Table("moderation_warnings").Where("guild_id = ?", parsed.GuildData.GS.ID).Count(&count)

			out += "```\n" + fmt.Sprintf("Total Server Warnings: `%d`", count)

			embed.Description = out

			return embed, nil

		}),
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "GiveRole",
		Aliases:       []string{"grole", "arole", "addrole"},
		Description:   "Gives a role to the specified member, with optional expiry",

		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Role", Type: &commands.RoleArg{}},
			{Name: "Duration", Type: &commands.DurationArg{}, Default: time.Duration(0)},
		},
		RequiredDiscordPermsHelp: "ManageRoles or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionManageRoles}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageRoles, config.GiveRoleCmdRoles, config.GiveRoleCmdEnabled)
			if err != nil {
				return nil, err
			}

			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			role := parsed.Args[1].Value.(*discordgo.Role)
			if role == nil {
				return "Couldn't find the specified role", nil
			}

			if !bot.IsMemberAboveRole(parsed.GuildData.GS, parsed.GuildData.MS, role) {
				return "Can't give roles above you", nil
			}

			dur := parsed.Args[2].Value.(time.Duration)

			// no point if the user has the role and is not updating the expiracy
			if common.ContainsInt64Slice(member.Member.Roles, role.ID) && dur <= 0 {
				return "That user already has that role", nil
			}

			err = common.AddRoleDS(member, role.ID)
			if err != nil {
				return nil, err
			}

			// schedule the expiry
			if dur > 0 {
				err := scheduledevents2.ScheduleRemoveRole(parsed.Context(), parsed.GuildData.GS.ID, target.ID, role.ID, time.Now().Add(dur))
				if err != nil {
					return nil, err
				}
			}

			// cancel the event to add the role
			scheduledevents2.CancelAddRole(parsed.Context(), parsed.GuildData.GS.ID, parsed.Author.ID, role.ID)

			action := MAGiveRole
			action.Prefix = "Gave the role " + role.Name + " to "
			if config.GiveRoleCmdModlog && config.IntActionChannel() != 0 {
				if dur > 0 {
					action.Footer = "Duration: " + common.HumanizeDuration(common.DurationPrecisionMinutes, dur)
				}
				CreateModlogEmbed(config, parsed.Author, action, target, "", "")
			}

			return GenericCmdResp(action, target, dur, true, dur <= 0), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "RemoveRole",
		Aliases:       []string{"rrole", "takerole", "trole"},
		Description:   "Removes the specified role from the target",

		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Role", Type: &commands.RoleArg{}},
		},
		RequiredDiscordPermsHelp: "ManageRoles or ManageServer",
		RequireBotPerms:          [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageServer}, {discordgo.PermissionManageRoles}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, target, err := MBaseCmd(parsed, parsed.Args[0].Int64())
			if err != nil {
				return nil, err
			}

			_, err = MBaseCmdSecond(parsed, "", true, discordgo.PermissionManageRoles, config.GiveRoleCmdRoles, config.GiveRoleCmdEnabled)
			if err != nil {
				return nil, err
			}

			member, err := bot.GetMember(parsed.GuildData.GS.ID, target.ID)
			if err != nil || member == nil {
				return "Member not found", err
			}

			role := parsed.Args[1].Value.(*discordgo.Role)
			if role == nil {
				return "Couldn't find the specified role", nil
			}

			if !bot.IsMemberAboveRole(parsed.GuildData.GS, parsed.GuildData.MS, role) {
				return "Can't remove roles above you", nil
			}

			err = common.RemoveRoleDS(member, role.ID)
			if err != nil {
				return nil, err
			}

			// cancel the event to remove the role
			scheduledevents2.CancelRemoveRole(parsed.Context(), parsed.GuildData.GS.ID, parsed.Author.ID, role.ID)

			action := MARemoveRole
			action.Prefix = "Removed the role " + role.Name + " from "
			if config.GiveRoleCmdModlog && config.IntActionChannel() != 0 {
				CreateModlogEmbed(config, parsed.Author, action, target, "", "")
			}

			return GenericCmdResp(action, target, 0, true, true), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "SetSlowmode",
		Aliases:       []string{"slow", "setslow", "slowmode"},
		Description:   "Sets the slowmode interval in the current channel",
		RequiredArgs:  1,
		Arguments: []*dcmd.ArgDef{
			{Name: "Interval", Help: "The interval of the slowmode", Type: &commands.DurationArg{}},
			{Name: "Channel", Help: "The channel to set the slowmode in", Type: dcmd.Channel},
		},
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			channelID := parsed.ChannelID
			interval := parsed.Args[0].Value.(time.Duration)
			rl := int(interval.Seconds())
			humanizedInterval := common.HumanizeDuration(common.DurationPrecisionSeconds, interval)

			if c := parsed.Args[1]; c.Value != nil {
				channelID = c.Value.(*dstate.ChannelState).ID
			}

			edit := &discordgo.ChannelEdit{
				RateLimitPerUser: &rl,
			}

			_, err := common.BotSession.ChannelEditComplex(channelID, edit)
			if err != nil {
				return nil, err
			}

			if rl == 0 {
				humanizedInterval = "none"
			}

			return "Slow mode in <#" + strconv.FormatInt(channelID, 10) + "> set to " + humanizedInterval, nil
		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "GetSlowmode",
		Aliases:                  []string{"getslow"},
		Description:              "Gets the slowmode interval in the current channel",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			channel, err := common.BotSession.Channel(parsed.ChannelID)

			if err != nil {
				return nil, err
			}

			time := strconv.FormatInt(int64(channel.RateLimitPerUser), 10)
			seconds := strings.Join([]string{time, "s"}, "")

			return templates.ToDuration(seconds).String(), nil
		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "AddToWatchList",
		Aliases:                  []string{"watch", "addwatch", "watchlist", "addwatchlist"},
		Description:              "Adds user to the watchlist",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to add to the watchlist", Type: &commands.MemberArg{}},
			{Name: "Reason", Help: "The reason for adding the user to the watchlist", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			user := parsed.Args[0].User()
			reason := parsed.Args[1].Str()
			var count int
			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}

			common.GORM.Model(&watchList).Count(&count)

			if count > 0 {
				common.GORM.Model(&watchList).First(&watchList)
				watchList.Reason = reason

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err := common.BotSession.ChannelMessageEditEmbed(1151502255703470101, watchList.MessageID, embed)
				if err != nil {
					message, err := common.BotSession.ChannelMessageSendEmbed(1151502255703470101, embed)
					if err != nil {
						return nil, err
					}

					watchList.MessageID = message.ID
				}

				err = common.GORM.Model(&watchList).Update(watchList).Error
				if err != nil {
					return nil, err
				}

				if parsed.TriggerType != 3 {
					err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
					if err != nil {
						return nil, err
					}
				}

				return fmt.Sprintf("%s's watchlist entry updated with reason: \"%s\"", user.Mention(), reason), nil
			} else {
				watchList := WatchList{
					GuildID:  guildID,
					UserID:   userID,
					AuthorID: strconv.FormatInt(parsed.Author.ID, 10),
					Reason:   reason,
				}

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				message, err := common.BotSession.ChannelMessageSendEmbed(1151502255703470101, embed)
				if err != nil {
					return nil, err
				}

				watchList.MessageID = message.ID

				err = common.GORM.Model(&watchList).Save(&watchList).Error
				if err != nil {
					return nil, err
				}

				if parsed.TriggerType != 3 {
					err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
					if err != nil {
						return nil, err
					}
				}

				return fmt.Sprintf("%s added to the watchlist with reason: \"%s\"", user.Mention(), reason), nil
			}
		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "RemoveFromWatchList",
		Aliases:                  []string{"rwatch", "removewatch", "rwatchlist", "removewatchlist"},
		Description:              "Removes user from the watchlist",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to add to the watchlist", Type: &commands.MemberArg{}}},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			user := parsed.Args[0].User()

			watchList := WatchList{UserID: uint64(user.ID)}
			var count int
			common.GORM.Model(&watchList).Count(&count)

			if count > 0 {
				common.GORM.Model(&watchList).First(&watchList)
				common.GORM.Model(&watchList).Association("Feuds").Delete()
				common.GORM.Model(&watchList).Association("VerbalWarnings").Delete()
				common.GORM.Model(&watchList).Delete(watchList)
				err := common.BotSession.ChannelMessageDelete(1151502255703470101, watchList.MessageID)
				log.Error(err)

				return fmt.Sprintf("%s removed from the watchlist", user.Mention()), nil
			} else {
				return "User hasn't been added to watchlist yet.", nil
			}
		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "AddHeadModeratorNote",
		Aliases:                  []string{"addwatchnote", "watchnote"},
		Description:              "Adds note to a watchlisted user",
		RequireDiscordPerms:      []int64{discordgo.PermissionAdministrator},
		RequiredDiscordPermsHelp: "Administrator",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user the note is for", Type: &commands.MemberArg{}},
			{Name: "Note", Help: "The note to add", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			user := parsed.Args[0].User()
			note := parsed.Args[1].Str()

			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}
			var count int

			common.GORM.Model(&watchList).Count(&count)

			if count > 0 {
				common.GORM.Model(&watchList).Preload("Feuds").Preload("VerbalWarnings").First(&watchList)

				watchList.HeadModeratorNote = note

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err := common.BotSession.ChannelMessageEditEmbed(1151502255703470101, watchList.MessageID, embed)
				if err != nil {
					message, _ := common.BotSession.ChannelMessageSendEmbed(1151502255703470101, embed)
					watchList.MessageID = message.ID
				}

				err = common.GORM.Model(&watchList).Update(watchList).Error
				if err != nil {
					return nil, err
				}

				if parsed.TriggerType != 3 {
					err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
					if err != nil {
						return nil, err
					}
				}

				return fmt.Sprintf("Head Moderator note added to %s's watchlist entry with value: \"%s\"", user.Mention(), note), nil
			}

			return "User hasn't been added to watchlist yet.", nil
		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "AddFeud",
		Aliases:                  []string{"feud"},
		Description:              "Adds a feud for the watchlisted user",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             4,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
			{Name: "FeudingUser", Help: "The user the feud is with", Type: &commands.MemberArg{}},
			{Name: "Reason", Help: "The reason for the feud.", Type: dcmd.String},
			{Name: "MessageLink", Help: "Message link to the feud", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			user := parsed.Args[0].User()
			feudingUser := parsed.Args[1].User()
			reason := parsed.Args[2].Str()
			messageLink := parsed.Args[3].Str()

			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}

			err := common.GORM.Model(&watchList).Preload("VerbalWarnings").Find(&watchList).Error
			if err != nil {
				return "User hasn't been added to watchlist yet.", err
			} else {
				var feuds []Feud
				count := common.GORM.Model(&WatchList{UserID: userID}).Association("Feuds").Count()

				if err != nil {
					return nil, err
				}

				feud := Feud{GuildID: guildID, AuthorID: parsed.Author.ID, Reason: reason, FeudingUserName: feudingUser.Username, MessageLink: messageLink}
				var parsedFeuds []Feud
				parsedFeuds = append(parsedFeuds, feud)

				if count > 0 {
					common.GORM.Model(&WatchList{UserID: userID}).Association("Feuds").Find(&feuds)

					for _, v := range feuds {
						if v.FeudingUserName != feudingUser.Username {
							parsedFeuds = append(parsedFeuds, v)
						}
					}

					err = common.GORM.Model(&WatchList{UserID: userID}).Association("Feuds").Replace(parsedFeuds).Error
				} else {
					err = common.GORM.Model(&WatchList{UserID: userID}).Association("Feuds").Replace(parsedFeuds).Error
				}
				if err != nil {
					return nil, err
				}

				watchList.Feuds = parsedFeuds

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err = common.BotSession.ChannelMessageEditEmbed(1151502255703470101, watchList.MessageID, embed)
				if err != nil {
					message, _ := common.BotSession.ChannelMessageSendEmbed(1151502255703470101, embed)

					watchList.MessageID = message.ID
				}

				if err != nil {
					return nil, err
				}

				if parsed.TriggerType != 3 {
					err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
					if err != nil {
						return nil, err
					}
				}

				return fmt.Sprintf("Feud between %s and %s has been added with reason: \"%s\"", user.Mention(), feudingUser.Mention(), reason), nil
			}

		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "AddVerbalWarning",
		Aliases:                  []string{"verbalwarning"},
		Description:              "Adds a verbal warning for the watchlisted user",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             3,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
			{Name: "Reason", Help: "The reason for the verbal warning.", Type: dcmd.String},
			{Name: "messageLink", Help: "The link to where the warning was given.", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			user := parsed.Args[0].User()
			reason := parsed.Args[1].Str()
			messageLink := parsed.Args[2].Str()

			userID := uint64(user.ID)

			watchList := WatchList{UserID: userID}
			err := common.GORM.Model(&watchList).Preload("Feuds").Find(&watchList).Error

			if err != nil {
				return "User hasn't been added to watchlist yet.", err
			} else {
				var verbalWarnings []VerbalWarning
				count := common.GORM.Model(&WatchList{UserID: userID}).Association("VerbalWarnings").Count()

				if err != nil {
					return nil, err
				}

				verbalWarning := VerbalWarning{GuildID: guildID, AuthorID: parsed.Author.ID, Reason: reason, MessageLink: messageLink}
				var parsedWarnings []VerbalWarning
				parsedWarnings = append(parsedWarnings, verbalWarning)

				if count > 0 {
					common.GORM.Model(&WatchList{UserID: userID}).Association("VerbalWarnings").Find(&verbalWarnings)
					for _, v := range verbalWarnings {
						if v.MessageLink != messageLink {
							parsedWarnings = append(parsedWarnings, v)
						}
					}

					err = common.GORM.Model(&WatchList{UserID: userID}).Association("VerbalWarnings").Replace(parsedWarnings).Error
				} else {
					err = common.GORM.Model(&WatchList{UserID: userID}).Association("VerbalWarnings").Append(parsedWarnings).Error
				}
				if err != nil {
					return nil, err
				}

				watchList.VerbalWarnings = parsedWarnings
				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err = common.BotSession.ChannelMessageEditEmbed(1151502255703470101, watchList.MessageID, embed)
				if err != nil {
					log.Error(err)
					message, err := common.BotSession.ChannelMessageSendEmbed(1151502255703470101, embed)
					if err != nil {
						return nil, err
					}
					watchList.MessageID = message.ID

					err = common.GORM.Model(&watchList).Update(watchList).Error
					if err != nil {
						return nil, err
					}
				}

				if err != nil {
					return nil, err
				}

				if parsed.TriggerType != 3 {
					err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
					if err != nil {
						return nil, err
					}
				}

				return fmt.Sprintf("Verbal warning for %s added to watchlist with reason: \"%s\"", user.Mention(), reason), nil
			}

		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "RefreshWatchList",
		Aliases:                  []string{"refeshwatch"},
		Description:              "Refreshes the watchlist entry",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			user := parsed.Args[0].User()
			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}
			err := common.GORM.Model(&watchList).Preload("Feuds").Preload("VerbalWarnings").Find(&watchList).Error

			if err != nil {
				return "User hasn't been added to watchlist yet.", err
			} else {
				common.GORM.Model(&watchList).First(&watchList)

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err := common.BotSession.ChannelMessageEditEmbed(1151502255703470101, watchList.MessageID, embed)
				if err != nil {
					message, err := common.BotSession.ChannelMessageSendEmbed(1151502255703470101, embed)
					if err != nil {
						return nil, err
					}

					watchList.MessageID = message.ID
					err = common.GORM.Model(&watchList).Update(watchList).Error
					if err != nil {
						return nil, err
					}
				}

				if parsed.TriggerType != 3 {
					err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
					if err != nil {
						return nil, err
					}
				}

				return fmt.Sprintf("%s's watchlist entry refreshed", user.Mention()), nil
			}
		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "ToggleWatchListPing",
		Aliases:                  []string{"toggleping"},
		Description:              "Toggles the watchlist ping to on or off",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
			{Name: "Toggle", Help: "Set to 1 for on, 0 for off.", Type: &dcmd.IntArg{Max: 1}, Default: 0},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			user := parsed.Args[0].User()
			pingable := parsed.Args[1].Bool()
			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}
			err := common.GORM.Model(&watchList).Find(&watchList).Error

			if err != nil {
				return "User hasn't been added to watchlist yet.", err
			} else {
				common.GORM.Model(&watchList).First(&watchList)

				watchList.Pingable = pingable
				watchList.LastPingedAt = time.Now()

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err := common.BotSession.ChannelMessageEditEmbed(1151502255703470101, watchList.MessageID, embed)
				if err != nil {
					message, err := common.BotSession.ChannelMessageSendEmbed(1151502255703470101, embed)
					if err != nil {
						return nil, err
					}

					watchList.MessageID = message.ID
				}

				err = common.GORM.Model(&watchList).Update(watchList).Error
				if err != nil {
					return nil, err
				}

				if parsed.TriggerType != 3 {
					err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
					if err != nil {
						return nil, err
					}
				}

				return fmt.Sprintf("%s's ping toggle set to %s", user.Mention(), strconv.FormatBool(pingable)), nil
			}
		},
	},
	{
		CustomEnabled:            true,
		CmdCategory:              commands.CategoryModeration,
		Name:                     "PingActiveWatchlistedUser",
		Aliases:                  []string{"watchlistping"},
		Description:              "Ping on duty in the watchlist channel",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		RequireBotPerms:          [][]int64{{discordgo.PermissionManageChannels}},
		SlashCommandEnabled:      true,
		DefaultEnabled:           true,
		IsResponseEphemeral:      true,
		RequiredArgs:             2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
			{Name: "ActiveChannelID", Help: "Channel ID of the channel the user is active in", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			user := parsed.Args[0].User()
			channelID := parsed.Args[1].Str()
			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}
			err := common.GORM.Model(&watchList).Find(&watchList).Error

			if err != nil {
				return "User hasn't been added to watchlist yet.", err
			} else {
				common.GORM.Model(&watchList).First(&watchList)

				if watchList.Pingable {
					if time.Since(watchList.LastPingedAt).Hours() > 2 {
						message, err := common.BotSession.ChannelMessageSend(1151502255703470101, fmt.Sprintf("<@&1142548147600638022>\n Watchlisted member %s is active in channel: <#%s>", user.Mention(), channelID))
						if err != nil {
							return nil, err
						}
						watchList.LastPingedAt = time.Now()

						go func() {
							time.Sleep(10 * time.Minute)
							err := common.BotSession.ChannelMessageDelete(message.ChannelID, message.ID)
							if err != nil {
								return
							}
						}()

						err = common.GORM.Model(&watchList).Update(watchList).Error
						if err != nil {
							return nil, err
						}

						return "Done", nil
					} else {
						return fmt.Sprintf("%s pinged too recently", user.Mention()), nil
					}
				} else {
					return fmt.Sprintf("%s's ping toggle set to false", user.Mention()), nil
				}
			}
		},
	},
}

func generatePunishments(data *LightDBEntry) []*discordgo.MessageEmbedField {
	var punishments []*discordgo.MessageEmbedField
	values, _ := templates.StringKeyDictionary(data.Value.(*templates.SDict))

	for key, value := range values {
		sDict := value.(*templates.SDict)
		reason := sDict.Get("r").(string)
		var duration string

		if sDict.HasKey("d") {
			seconds := sDict.Get("d").(int64)
			duration = strings.Join([]string{strconv.FormatInt(seconds, 10), "s"}, "")

		}

		field := []*discordgo.MessageEmbedField{
			{Name: "ID", Value: key, Inline: true},
			{Name: "Reason", Value: reason, Inline: true},
			{Name: "Duration", Value: templates.ToDuration(duration).String(), Inline: true},
		}

		punishments = append(punishments, field...)
	}

	return punishments
}

func generateWatchlistEmbed(guildID int64, user *discordgo.User, author *discordgo.User, data WatchList) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title: user.Globalname,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: discordgo.EndpointUserAvatar(user.ID, user.Avatar),
		},
		Color:       16777215,
		Description: fmt.Sprintf(">>> **User:** %s\n**Reason:** %s\n **Ping Toggle:** %s", user.Mention(), data.Reason, strconv.FormatBool(data.Pingable)),
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Watchlist entry last updated by %s", author.Globalname),
			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
		},
	}

	hasLogs, err := dbHasKey(guildID, user.ID, "logs")
	if err != nil {
		log.Error(err)
		return nil
	}

	if hasLogs {
		result, err := dbGet(guildID, user.ID, "logs")
		if err != nil {
			log.Error(err)
		} else {
			embed.Fields = append(embed.Fields, generatePunishments(result)...)
		}
	}

	if len(data.Feuds) > 0 {
		for _, feud := range data.Feuds {
			if err != nil {
				log.Error(err)
			} else {
				fields := []*discordgo.MessageEmbedField{
					{Name: "Feuding User", Value: feud.FeudingUserName, Inline: true},
					{Name: "Reason", Value: feud.Reason, Inline: true},
					{Name: "Link", Value: feud.MessageLink, Inline: true},
				}
				embed.Fields = append(embed.Fields, fields...)
			}
		}
	}

	if len(data.VerbalWarnings) > 0 {
		for _, verbalWarning := range data.VerbalWarnings {
			if verbalWarning.AuthorID != 0 {
				authorMember, err := bot.GetMember(guildID, verbalWarning.AuthorID)
				if err != nil {
					log.Error(err, guildID, verbalWarning)
				} else {
					fields := []*discordgo.MessageEmbedField{
						{Name: "Verbal Warning", Value: verbalWarning.Reason, Inline: true},
						{Name: "Given By", Value: authorMember.User.Mention(), Inline: true},
						{Name: "Link", Value: verbalWarning.MessageLink, Inline: true},
					}
					embed.Fields = append(embed.Fields, fields...)
				}
			}
		}
	}

	if len(strings.Fields(data.HeadModeratorNote)) > 0 {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: "Head Moderator Note", Value: data.HeadModeratorNote})
	}

	return embed
}

func AdvancedDeleteMessages(guildID, channelID int64, triggerID int64, filterUser int64, regex string, invertRegexMatch bool, toID int64, fromID int64, maxAge time.Duration, minAge time.Duration, pinFilterEnable bool, attachmentFilterEnable bool, deleteNum, fetchNum int) (int, error) {
	var compiledRegex *regexp.Regexp
	if regex != "" {
		// Start by compiling the regex
		var err error
		compiledRegex, err = regexp.Compile(regex)
		if err != nil {
			return 0, err
		}
	}

	var pinnedMessages map[int64]struct{}
	if pinFilterEnable {
		//Fetch pinned messages from channel and make a map with ids as keys which will make it easy to verify if a message with a given ID is pinned message
		messageSlice, err := common.BotSession.ChannelMessagesPinned(channelID)
		if err != nil {
			return 0, err
		}
		pinnedMessages = make(map[int64]struct{}, len(messageSlice))
		for _, msg := range messageSlice {
			pinnedMessages[msg.ID] = struct{}{} //empty struct works because we are not really interested in value
		}
	}

	msgs, err := bot.GetMessages(guildID, channelID, fetchNum, false)
	if err != nil {
		return 0, err
	}

	toDelete := make([]int64, 0)
	now := time.Now()
	for i := 0; i < len(msgs); i++ {
		if msgs[i].ID == triggerID {
			continue
		}

		if filterUser != 0 && msgs[i].Author.ID != filterUser {
			continue
		}

		// Can only bulk delete messages up to 2 weeks (but add 1 minute buffer account for time sync issues and other smallies)
		if now.Sub(msgs[i].ParsedCreatedAt) > (time.Hour*24*14)-time.Minute {
			continue
		}

		// Check regex
		if compiledRegex != nil {
			ok := compiledRegex.MatchString(msgs[i].Content)
			if invertRegexMatch {
				ok = !ok
			}
			if !ok {
				continue
			}
		}

		// Check max age
		if maxAge != 0 && now.Sub(msgs[i].ParsedCreatedAt) > maxAge {
			continue
		}

		// Check min age
		if minAge != 0 && now.Sub(msgs[i].ParsedCreatedAt) < minAge {
			continue
		}

		// Check if pinned message to ignore
		if pinFilterEnable {
			if _, found := pinnedMessages[msgs[i].ID]; found {
				continue
			}
		}

		// Continue only if current msg ID is > fromID
		if fromID > 0 && fromID < msgs[i].ID {
			continue
		}

		// Continue only if current msg ID is < toID
		if toID > 0 && toID > msgs[i].ID {
			continue
		}

		// Check whether to ignore messages without attachments
		if attachmentFilterEnable && len(msgs[i].Attachments) == 0 {
			continue
		}

		toDelete = append(toDelete, msgs[i].ID)

		if len(toDelete) >= deleteNum || len(toDelete) >= 100 {
			break
		}
	}

	if len(toDelete) < 1 {
		return 0, nil
	} else if len(toDelete) == 1 {
		err = common.BotSession.ChannelMessageDelete(channelID, toDelete[0])
	} else {
		err = common.BotSession.ChannelMessagesBulkDelete(channelID, toDelete)
	}

	return len(toDelete), err
}

func PaginateWarnings(parsed *dcmd.Data) func(p *paginatedmessages.PaginatedMessage, page int) (*discordgo.MessageEmbed, error) {

	return func(p *paginatedmessages.PaginatedMessage, page int) (*discordgo.MessageEmbed, error) {

		var err error
		skip := (page - 1) * 6
		userID := parsed.Args[0].Int64()
		limit := 6

		var result []*WarningModel
		var count int
		err = common.GORM.Table("moderation_warnings").Where("user_id = ? AND guild_id = ?", userID, parsed.GuildData.GS.ID).Count(&count).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, err
		}
		err = common.GORM.Where("user_id = ? AND guild_id = ?", userID, parsed.GuildData.GS.ID).Order("id desc").Offset(skip).Limit(limit).Find(&result).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, err
		}

		if len(result) < 1 && p != nil && p.LastResponse != nil { //Dont send No Results error on first execution
			return nil, paginatedmessages.ErrNoResults
		}

		desc := fmt.Sprintf("**Total :** `%d`", count)
		var fields []*discordgo.MessageEmbedField
		currentField := &discordgo.MessageEmbedField{
			Name:  "⠀", //Use braille blank character for seamless transition between feilds
			Value: "",
		}
		fields = append(fields, currentField)
		if len(result) > 0 {

			for _, entry := range result {

				entry_formatted := fmt.Sprintf("#%d: <t:%d:f> - By: **%s** (%13s) \n **Reason:** %s", entry.ID, entry.CreatedAt.Unix(), entry.AuthorUsernameDiscrim, entry.AuthorID, entry.Message)
				if len([]rune(entry_formatted)) > 900 {
					entry_formatted = common.CutStringShort(entry_formatted, 900)
				}
				entry_formatted += "\n"
				purgedWarnLogs := logs.ConfEnableMessageLogPurge.GetBool() && entry.CreatedAt.Before(time.Now().AddDate(0, 0, -30))
				if entry.LogsLink != "" && !purgedWarnLogs {
					entry_formatted += fmt.Sprintf("> logs: [`link`](%s)\n", entry.LogsLink)
				}
				if len([]rune(currentField.Value+entry_formatted)) > 1023 {
					currentField = &discordgo.MessageEmbedField{
						Name:  "⠀",
						Value: entry_formatted + "\n",
					}
					fields = append(fields, currentField)
				} else {
					currentField.Value += entry_formatted + "\n"
				}
			}

		} else {
			currentField.Value = "No Warnings"
		}

		return &discordgo.MessageEmbed{
			Title:       fmt.Sprintf("Warnings - User : %d", userID),
			Description: desc,
			Fields:      fields,
		}, nil
	}
}

func serializeValue(v interface{}) ([]byte, error) {
	var b bytes.Buffer
	enc := msgpack.NewEncoder(templates.LimitWriter(&b, 100000))
	err := enc.Encode(v)
	b.Bytes()
	return b.Bytes(), err
}

func limitString(s string, l int) string {
	if len(s) <= l {
		return s
	}

	lastValidLoc := 0
	for i := range s {
		if i > l {
			break
		}
		lastValidLoc = i
	}

	return s[:lastValidLoc]
}
func dbGet(guildID int64, userID int64, key interface{}) (*LightDBEntry, error) {
	keyStr := limitString(templates.ToString(key), 256)
	m, err := models.TemplatesUserDatabases(qm.Where("guild_id = ? AND user_id = ? AND key = ? AND (expires_at IS NULL OR expires_at > now())", guildID, userID, keyStr)).OneG(context.Background())

	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}

		return nil, nil
	}

	return ToLightDBEntry(m)
}

func dbHasKey(guildID int64, userID int64, key interface{}) (bool, error) {
	keyStr := limitString(templates.ToString(key), 256)
	count, err := models.TemplatesUserDatabases(qm.Where("guild_id = ? AND user_id = ? AND key = ? AND (expires_at IS NULL OR expires_at > now())", guildID, userID, keyStr)).CountG(context.Background())

	if err != nil {
		if err != sql.ErrNoRows {
			return false, err
		}

		return false, nil
	}

	return count > 0, nil
}

func ToLightDBEntry(m *models.TemplatesUserDatabase) (*LightDBEntry, error) {
	var dst interface{}
	dec := newDecoder(bytes.NewBuffer(m.ValueRaw))
	err := dec.Decode(&dst)
	if err != nil {
		return nil, err
	}

	decodedValue := dst
	if common.IsNumber(dst) {
		decodedValue = m.ValueNum
	}

	entry := &LightDBEntry{
		ID:      m.ID,
		GuildID: m.GuildID,
		UserID:  m.UserID,

		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,

		Key:   m.Key,
		Value: decodedValue,

		ExpiresAt: m.ExpiresAt.Time,
	}
	entry.User.ID = entry.UserID

	return entry, nil
}

func newDecoder(buf *bytes.Buffer) *msgpack.Decoder {
	dec := msgpack.NewDecoder(buf)

	dec.SetDecodeMapFunc(func(d *msgpack.Decoder) (interface{}, error) {
		n, err := d.DecodeMapLen()
		if err != nil {
			return nil, err
		}

		isStringKeysOnly := true
		mi := make(map[interface{}]interface{}, n)
		ms := make(map[string]interface{})

		for i := 0; i < n; i++ {
			mk, err := d.DecodeInterface()
			if err != nil {
				return nil, err
			}

			mv, err := d.DecodeInterface()
			if err != nil {
				return nil, err
			}

			// if the map only has string keys, use a map[string]interface{}
			if isStringKeysOnly {
				if s, ok := mk.(string); ok {
					// so far only string keys
					ms[s] = mv
				} else {
					// copy over the map to the interface{} keyed one
					isStringKeysOnly = false
					for jk, jv := range ms {
						mi[jk] = jv
					}
					mi[mk] = mv
				}
			} else {
				mi[mk] = mv
			}
		}
		if isStringKeysOnly {
			return ms, nil
		}

		return mi, nil
	})

	return dec
}
