package moderation

import (
	"emperror.dev/errors"
	"fmt"
	"github.com/cirelion/flint/analytics"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/bot/paginatedmessages"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/common/scheduledevents2"
	"github.com/cirelion/flint/common/templates"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	"github.com/cirelion/flint/web"
	log "github.com/sirupsen/logrus"
	"math"
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

type ModLogEntry struct {
	ID       uint64
	Type     string
	Author   discordgo.User
	Reason   string
	LogLink  string
	Duration time.Duration
	GivenAt  time.Time
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
		RequiredArgs:  3,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Duration", Type: &commands.DurationArg{}, Default: time.Duration(0)},
			{Name: "Reason", Type: dcmd.String},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "ddays", Help: "Number of days of messages to delete", Type: dcmd.Int},
		},
		RequiredDiscordPermsHelp:  "BanMembers or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionBanMembers}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			var proof string
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
				proof, err = getMessageReferenceContent(parsed.TraditionalTriggerData)
			}
			err = BanUserWithDuration(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, proof, target, banDuration, ddays)
			if err != nil {
				return nil, err
			}

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			return generateGenericModEmbed(MABanned, parsed.Author, target, reason, "", "", banDuration, config.GuildID), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Unban",
		Aliases:       []string{"unbanid"},
		Description:   "Unbans a user. Reason requirement is same as ban command setting.",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp:  "BanMembers or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionBanMembers}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       false,
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

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			return generateGenericModEmbed(MAUnbanned, parsed.Author, target, reason, "", "", 0, config.GuildID), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Kick",
		Description:   "Kicks a member",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp: "KickMembers or ManageServer",
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "cl", Help: "Messages to delete", Type: &dcmd.IntArg{Min: 1, Max: 100}},
		},
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionKickMembers}},
		ApplicationCommandEnabled: true,
		IsResponseEphemeral:       false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			var proof string
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
				proof, err = getMessageReferenceContent(parsed.TraditionalTriggerData)
			}

			err = KickUser(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, proof, target, toDel)
			if err != nil {
				return nil, err
			}

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			return generateGenericModEmbed(MAKick, parsed.Author, target, reason, "", "", -1, config.GuildID), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Mute",
		Description:   "Mutes a member",
		RequiredArgs:  3,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Duration", Type: &commands.DurationArg{}},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp:  "KickMembers or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionManageRoles}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			var proof string
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
				proof, err = getMessageReferenceContent(parsed.TraditionalTriggerData)
			}

			err = MuteUnmuteUser(config, true, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, proof, member, int(d.Minutes()))
			if err != nil {
				return nil, err
			}

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			common.BotSession.GuildMemberMove(parsed.GuildData.GS.ID, target.ID, 0)
			return generateGenericModEmbed(MAMute, parsed.Author, target, reason, "", "", d, config.GuildID), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Unmute",
		Description:   "Unmutes a member",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp:  "KickMembers or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionManageRoles}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       false,
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
			err = MuteUnmuteUser(config, false, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, "", member, -1)
			if err != nil {
				return nil, err
			}

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			return generateGenericModEmbed(MAUnmute, parsed.Author, target, reason, "", "", -1, config.GuildID), nil
		},
	},
	{
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "Timeout",
		Description:   "Timeout a member",
		Aliases:       []string{"to"},
		RequiredArgs:  3,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Duration", Type: &commands.DurationArg{}},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp:  "TimeoutMembers/ModerateMembers or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionModerateMembers}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			var proof string
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

			if parsed.TriggerType == 2 {
				proof, err = getMessageReferenceContent(parsed.TraditionalTriggerData)
			}

			err = TimeoutUser(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, reason, proof, &member.User, d)
			if err != nil {
				return nil, err
			}

			return generateGenericModEmbed(MATimeoutAdded, parsed.Author, target, reason, "", "", d, config.GuildID), nil
		},
	}, {
		CustomEnabled: true,
		CmdCategory:   commands.CategoryModeration,
		Name:          "RemoveTimeout",
		Aliases:       []string{"untimeout", "cleartimeout", "deltimeout", "rto"},
		Description:   "Removes a member's timeout",
		RequiredArgs:  2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Type: dcmd.UserID},
			{Name: "Reason", Type: dcmd.String},
		},
		RequiredDiscordPermsHelp:  "TimeoutMember/ModerateMember or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionModerateMembers}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       false,
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

			return generateGenericModEmbed(MATimeoutRemoved, parsed.Author, target, reason, "", "", -1, config.GuildID), nil
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
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       true,
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
		RequiredDiscordPermsHelp:  "ManageMessages or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionManageMessages}},
		ArgumentCombos:            [][]int{{0}, {0, 1}, {1, 0}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       true,
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

			// Wait a second so the client dosen't glitch out
			time.Sleep(time.Second)

			numDeleted, err := AdvancedDeleteMessages(parsed.GuildData.GS.ID, parsed.ChannelID, triggerID, userFilter, re, invertRegexMatch, toID, fromID, ma, minAge, pe, attachments, num, limitFetch)
			deleteMessageWord := "messages"
			if numDeleted == 1 {
				deleteMessageWord = "message"
			}

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			return dcmd.NewTemporaryResponse(time.Second*5, fmt.Sprintf("Deleted %d %s! :')", numDeleted, deleteMessageWord), true), err
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
		RequiredDiscordPermsHelp:  "ManageMessages or ManageServer",
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
		IsResponseEphemeral:       false,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			var proof string
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
				proof, err = getMessageReferenceContent(parsed.TraditionalTriggerData)
			}

			err = WarnUser(config, parsed.GuildData.GS.ID, parsed.GuildData.CS, msg, parsed.Author, target, parsed.Args[1].Str(), proof)
			if err != nil {
				return nil, err
			}

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			return generateGenericModEmbed(MAWarned, parsed.Author, target, parsed.Args[1].Str(), "", "", 4*7*24*time.Hour, config.GuildID), nil
		},
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
		RequiredDiscordPermsHelp:  "ManageRoles or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionManageRoles}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
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
				CreateModlogEmbed(config, parsed.Author, action, target, "", "", "", -1)
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
		RequiredDiscordPermsHelp:  "ManageRoles or ManageServer",
		RequireBotPerms:           [][]int64{{discordgo.PermissionAdministrator}, {discordgo.PermissionManageGuild}, {discordgo.PermissionManageRoles}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            false,
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
				CreateModlogEmbed(config, parsed.Author, action, target, "", "", "", -1)
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
		RequireDiscordPerms:       []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp:  "ManageMessages",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
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
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "GetSlowmode",
		Aliases:                   []string{"getslow"},
		Description:               "Gets the slowmode interval in the current channel",
		RequireDiscordPerms:       []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp:  "ManageMessages",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
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
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "AddToWatchList",
		Aliases:                   []string{"watch", "addwatch", "watchlist", "addwatchlist"},
		Description:               "Adds user to the watchlist",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to add to the watchlist", Type: &commands.MemberArg{}},
			{Name: "Reason", Help: "The reason for adding the user to the watchlist", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			config, _ := GetConfig(guildID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
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
				_, err := common.BotSession.ChannelMessageEditEmbed(int64(watchListChannel), watchList.MessageID, embed)
				if err != nil {
					message, err := common.BotSession.ChannelMessageSendEmbed(int64(watchListChannel), embed)
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
					Ping:     "false",
				}

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				message, err := common.BotSession.ChannelMessageSendEmbed(int64(watchListChannel), embed)
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
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "RemoveFromWatchList",
		Aliases:                   []string{"rwatch", "removewatch", "rwatchlist", "removewatchlist"},
		Description:               "Removes user from the watchlist",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to remove from the watchlist", Type: dcmd.UserID},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _ := GetConfig(parsed.GuildData.GS.ID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
			userID := parsed.Args[0].Int64()

			watchList := WatchList{UserID: uint64(userID)}

			var count int
			common.GORM.Model(&watchList).Count(&count)

			if count > 0 {
				common.GORM.Model(&watchList).First(&watchList)
				common.GORM.Model(&watchList).Association("Feuds").Delete()
				common.GORM.Model(&watchList).Association("VerbalWarnings").Delete()
				common.GORM.Model(&watchList).Delete(watchList)
				err := common.BotSession.ChannelMessageDelete(int64(watchListChannel), watchList.MessageID)

				if err != nil {
					return nil, err
				}

				return "User removed from the watchlist.", nil
			} else {
				return "User hasn't been added to watchlist yet.", nil
			}
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "AddFeud",
		Aliases:                   []string{"feud"},
		Description:               "Adds a feud for the watchlisted user",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              4,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
			{Name: "FeudingUser", Help: "The user the feud is with", Type: &commands.MemberArg{}},
			{Name: "Reason", Help: "The reason for the feud.", Type: dcmd.String},
			{Name: "MessageLink", Help: "Message link to the feud", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			config, _ := GetConfig(guildID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
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

				_, err = common.BotSession.ChannelMessageEditEmbed(int64(watchListChannel), watchList.MessageID, embed)
				if err != nil {
					message, _ := common.BotSession.ChannelMessageSendEmbed(int64(watchListChannel), embed)

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
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "AddVerbalWarning",
		Aliases:                   []string{"verbalwarning"},
		Description:               "Adds a verbal warning for the watchlisted user",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              3,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
			{Name: "Reason", Help: "The reason for the verbal warning.", Type: dcmd.String},
			{Name: "messageLink", Help: "The link to where the warning was given.", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			config, _ := GetConfig(guildID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
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

				_, err = common.BotSession.ChannelMessageEditEmbed(int64(watchListChannel), watchList.MessageID, embed)
				if err != nil {
					log.Error(err)
					message, err := common.BotSession.ChannelMessageSendEmbed(int64(watchListChannel), embed)
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
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "AddHeadModeratorNote",
		Aliases:                   []string{"addwatchnote", "watchnote"},
		Description:               "Adds note to a watchlisted user",
		RequireDiscordPerms:       []int64{discordgo.PermissionAdministrator},
		RequiredDiscordPermsHelp:  "Administrator",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user the note is for", Type: &commands.MemberArg{}},
			{Name: "Note", Help: "The note to add", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _ := GetConfig(parsed.GuildData.GS.ID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
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

				_, err := common.BotSession.ChannelMessageEditEmbed(int64(watchListChannel), watchList.MessageID, embed)
				if err != nil {
					message, _ := common.BotSession.ChannelMessageSendEmbed(int64(watchListChannel), embed)
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
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "RefreshWatchList",
		Aliases:                   []string{"refeshwatch"},
		Description:               "Refreshes the watchlist entry",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			config, _ := GetConfig(guildID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
			user := parsed.Args[0].User()
			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}
			var count int

			common.GORM.Model(&watchList).Count(&count)

			if count > 0 {
				common.GORM.Model(&watchList).Preload("Feuds").Preload("VerbalWarnings").First(&watchList)
				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err := common.BotSession.ChannelMessageEditEmbed(int64(watchListChannel), watchList.MessageID, embed)
				if err != nil {
					message, err := common.BotSession.ChannelMessageSendEmbed(int64(watchListChannel), embed)
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

			return "User hasn't been added to watchlist yet.", nil
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "ToggleWatchListPing",
		Aliases:                   []string{"toggleping"},
		Description:               "Toggles the watchlist ping to on or off",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			config, _ := GetConfig(guildID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
			user := parsed.Args[0].User()
			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}
			var count int

			common.GORM.Model(&watchList).Count(&count)

			if count > 0 {
				common.GORM.Model(&watchList).Preload("Feuds").Preload("VerbalWarnings").First(&watchList)
				if watchList.Ping == "false" {
					watchList.Ping = "true"
				} else {
					watchList.Ping = "false"
				}

				watchList.LastPingedAt = time.Now()

				embed := generateWatchlistEmbed(guildID, user, parsed.Author, watchList)

				_, err := common.BotSession.ChannelMessageEditEmbed(int64(watchListChannel), watchList.MessageID, embed)
				if err != nil {
					message, _ := common.BotSession.ChannelMessageSendEmbed(int64(watchListChannel), embed)
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

				return fmt.Sprintf("%s's ping toggle set to %s", user.Mention(), strconv.FormatBool(watchList.Ping == "true")), nil
			}

			return "User hasn't been added to watchlist yet.", nil
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "PingActiveWatchlistedUser",
		Aliases:                   []string{"watchlistping"},
		Description:               "Ping on duty in the watchlist channel",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              2,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The watchlisted user", Type: &commands.MemberArg{}},
			{Name: "ActiveChannelID", Help: "Channel ID of the channel the user is active in", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			config, _ := GetConfig(parsed.GuildData.GS.ID)
			watchListChannel, _ := strconv.Atoi(config.WatchListChannel)
			user := parsed.Args[0].User()
			channelID := parsed.Args[1].Str()
			userID := uint64(user.ID)
			watchList := WatchList{UserID: userID}
			err := common.GORM.Model(&watchList).Find(&watchList).Error

			if err != nil {
				return "User hasn't been added to watchlist yet.", nil
			} else if watchList.Ping == "true" {
				if time.Since(watchList.LastPingedAt).Hours() > 2 {
					message, err := common.BotSession.ChannelMessageSendComplex(int64(watchListChannel), &discordgo.MessageSend{
						AllowedMentions: discordgo.AllowedMentions{Parse: []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeRoles}},
						Content:         fmt.Sprintf("<@&1142548147600638022>\n Watchlisted member %s is active in channel: <#%s>", user.Mention(), channelID),
					})

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
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "Logs",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		ApplicationCommandType:    2,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			userID := parsed.SlashCommandTriggerData.Interaction.DataCommand.TargetID
			member, err := bot.GetMember(guildID, userID)
			punishList, err := generatePunishList(guildID, uint64(userID))
			if err != nil {
				return nil, err
			}

			if len(punishList) < 1 {
				return "User has no log entries.", nil
			}

			maxPage := int(math.Ceil(float64(len(punishList)) / float64(5)))
			_, err = paginatedmessages.CreatePaginatedModLogMessage(parsed.Session, parsed.SlashCommandTriggerData.Interaction.Token, parsed.GuildData.GS.ID, parsed.ChannelID, 1, maxPage, func(p *paginatedmessages.PaginatedMessage, page int) (*discordgo.MessageEmbed, error) {
				return modLogPager(member, p, page, punishList)
			})

			return nil, err
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "PunishLogs",
		Aliases:                   []string{"warninfo", "muteinfo", "kickinfo", "baninfo"},
		Description:               "View punishment info of the user",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              1,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to remove from the watchlist", Type: dcmd.UserID},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			userID := parsed.Args[0].Int64()

			member, err := bot.GetMember(guildID, userID)

			punishList, err := generatePunishList(guildID, uint64(userID))
			if err != nil {
				return nil, err
			}

			if len(punishList) < 1 {
				return "User has no log entries.", nil
			}

			maxPage := int(math.Ceil(float64(len(punishList)) / float64(5)))
			_, err = paginatedmessages.CreatePaginatedModLogMessage(parsed.Session, parsed.SlashCommandTriggerData.Interaction.Token, parsed.GuildData.GS.ID, parsed.ChannelID, 1, maxPage, func(p *paginatedmessages.PaginatedMessage, page int) (*discordgo.MessageEmbed, error) {
				return modLogPager(member, p, page, punishList)
			})

			if parsed.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(parsed.ChannelID, parsed.TraditionalTriggerData.Message.ID)
				if err != nil {
					return nil, err
				}
			}

			return nil, err
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "EditLog",
		Description:               "Edits a mod log entry of the user",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              4,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user you want to update the punishments of", Type: &commands.MemberArg{}},
			{Name: "Type", Help: "The type of punishment (Warn, Mute, Kick, Ban)", Type: dcmd.String},
			{Name: "ID", Help: "The punishment ID, found in the modLogs", Type: dcmd.Int},
			{Name: "UpdatedReason", Help: "The update reason for the punishment", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			userID := parsed.Args[0].User().ID
			Type := strings.Title(parsed.Args[1].Str())
			ID := parsed.Args[2].Int()
			reason := parsed.Args[3].Str()

			if !strings.Contains("WarnMuteKickBan", Type) {
				return "Incorrect type, must be one of: 'Warn', 'Mute', 'Kick', 'Ban'", nil
			}

			modLogs := ModLog{UserID: uint64(userID)}
			var err error

			switch Type {
			case "Warn":
				model := Warn{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Warns").Find(&model).Error
				if err != nil {
					return nil, err
				}

				model.Reason = reason
				err = common.GORM.Model(&modLogs).Association("Warns").Append(model).Error
			case "Mute":
				model := Mute{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Mutes").Find(&model).Error
				if err != nil {
					return nil, err
				}

				model.Reason = reason
				err = common.GORM.Model(&modLogs).Association("Mutes").Append(model).Error
			case "Kick":
				model := Kick{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Kicks").Find(&model).Error
				if err != nil {
					return nil, err
				}

				model.Reason = reason
				err = common.GORM.Model(&modLogs).Association("Kicks").Append(model).Error
			case "Ban":
				model := Ban{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Bans").Find(&model).Error
				if err != nil {
					return nil, err
				}

				model.Reason = reason
				err = common.GORM.Model(&modLogs).Association("Bans").Append(model).Error
			}

			if err != nil {
				return nil, err
			}

			return "Done.", nil
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "DeleteLog",
		Description:               "Deletes a mod log entry of the user",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              3,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user you want to delete the punishments of", Type: &commands.MemberArg{}},
			{Name: "Type", Help: "The type of punishment (Warn, Mute, Kick, Ban)", Type: dcmd.String},
			{Name: "ID", Help: "The punishment ID, found in the modLogs", Type: dcmd.Int},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			userID := parsed.Args[0].User().ID
			Type := strings.Title(parsed.Args[1].Str())
			ID := parsed.Args[2].Int()

			if !strings.Contains("WarnMuteKickBan", Type) {
				return "Incorrect type, must be one of: 'Warn', 'Mute', 'Kick', 'Ban'", nil
			}

			modLogs := ModLog{UserID: uint64(userID)}
			var err error

			switch Type {
			case "Warn":
				model := Warn{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Warns").Delete(&model).Error
			case "Mute":
				model := Mute{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Mutes").Delete(&model).Error
			case "Kick":
				model := Kick{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Kicks").Delete(&model).Error
			case "Ban":
				model := Ban{ID: uint(ID)}

				err = common.GORM.Model(&modLogs).Association("Bans").Delete(&model).Error
			}

			if err != nil {
				return nil, err
			}

			return "Done.", nil
		},
	},
	{
		CmdCategory:               commands.CategoryModeration,
		Name:                      "On Duty",
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		IsResponseEphemeral:       true,
		ApplicationCommandType:    3,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			config, err := GetConfig(guildID)
			member, err := bot.GetMember(guildID, parsed.Author.ID)

			if err != nil {
				return nil, err
			}
			onDuty := &OnDuty{UserID: uint64(member.User.ID)}
			err = common.GORM.Model(onDuty).FirstOrCreate(onDuty).Error
			if err != nil {
				return nil, err
			}

			if common.ContainsInt64Slice(member.Member.Roles, config.IntOnDutyRole()) {
				err = common.BotSession.GuildMemberRoleRemove(guildID, member.User.ID, config.IntOnDutyRole())
				onDuty.OnDuty = false
			} else {
				err = common.BotSession.GuildMemberRoleAdd(guildID, member.User.ID, config.IntOnDutyRole())
				onDuty.OnDuty = true
				onDuty.OnDutySetAt = time.Now()
			}

			if err != nil {
				return nil, err
			}

			err = common.GORM.Model(onDuty).Update(onDuty).Error
			if err != nil {
				return nil, err
			}

			if onDuty.OnDuty {
				return "You are now On Duty!", nil
			}

			return "You are now Off Duty!", nil
		},
	},
	{
		CmdCategory:               commands.CategoryModeration,
		Name:                      "Set Off Duty",
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		IsResponseEphemeral:       true,
		ApplicationCommandType:    2,
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			userID := parsed.SlashCommandTriggerData.Interaction.DataCommand.TargetID
			guildID := parsed.GuildData.GS.ID

			config, err := GetConfig(guildID)
			member, err := bot.GetMember(guildID, userID)
			if err != nil {
				return nil, err
			}

			onDuty := &OnDuty{UserID: uint64(member.User.ID)}
			err = common.GORM.Model(onDuty).First(onDuty).Error
			if err != nil {
				return "User has never gone On Duty.", err
			}

			if common.ContainsInt64Slice(member.Member.Roles, config.IntOnDutyRole()) {
				err = common.BotSession.GuildMemberRoleRemove(guildID, member.User.ID, config.IntOnDutyRole())
				onDuty.OnDuty = false
				if err != nil {
					return "Something went wrong trying to remove the role.", err
				}

				err = common.GORM.Model(onDuty).Update(onDuty).Error
				if err != nil {
					return nil, err
				}

				err = bot.SendDM(userID, fmt.Sprintf("You have been set Off Duty by %s", parsed.Author.Mention()))
				if err != nil {
					return "Error trying to send DM informing the member that was set Off Duty.", err
				}

				return fmt.Sprintf("%s set Off Duty!", member.User.Mention()), nil
			} else {
				return fmt.Sprintf("%s is already Off Duty!", member.User.Mention()), nil
			}
		},
	},
	{
		CustomEnabled:             true,
		CmdCategory:               commands.CategoryModeration,
		Name:                      "SetOnDutyDuration",
		Description:               "Sets the duration until you are automatically put off duty.",
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		ApplicationCommandEnabled: true,
		DefaultEnabled:            true,
		IsResponseEphemeral:       true,
		RequiredArgs:              1,
		Arguments: []*dcmd.ArgDef{
			{Name: "Duration", Type: &commands.DurationArg{}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			guildID := parsed.GuildData.GS.ID
			config, err := GetConfig(guildID)
			duration := parsed.Args[0].Value.(time.Duration)
			onDuty := &OnDuty{UserID: uint64(parsed.Author.ID)}

			err = common.GORM.Model(onDuty).FirstOrCreate(onDuty).Error
			if err != nil {
				return nil, err
			}

			onDuty.OnDuty = true
			onDuty.OnDutyDuration = duration
			onDuty.OnDutySetAt = time.Now()

			err = common.GORM.Model(onDuty).Update(&onDuty).Error
			err = common.BotSession.GuildMemberRoleAdd(parsed.GuildData.GS.ID, parsed.Author.ID, config.IntOnDutyRole())
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf("Set On Duty duration to %s", common.HumanizeDuration(common.DurationPrecisionHours, duration)), nil
		},
	},
}

type EphemeralOrGuild struct {
	Content string
	Embed   *discordgo.MessageEmbed
}

func getMessageReferenceContent(triggerData *dcmd.TraditionalTriggerData) (string, error) {
	messageReference := triggerData.Message.MessageReference

	if messageReference != nil {
		referencedMessage, err := common.BotSession.ChannelMessage(messageReference.ChannelID, messageReference.MessageID)
		if err != nil {
			return "", err
		}

		if len(referencedMessage.Attachments) > 0 {
			return referencedMessage.Attachments[0].URL, nil
		}

		return referencedMessage.Content, nil
	}

	return "No content", nil
}

func modLogPager(member *dstate.MemberState, p *paginatedmessages.PaginatedMessage, page int, modLogEntries []ModLogEntry) (*discordgo.MessageEmbed, error) {
	count := len(modLogEntries)
	if count < 1 && p != nil && p.LastResponse != nil { //Dont send No Results error on first execution
		return nil, paginatedmessages.ErrNoResults
	}

	maxNum := 5 * page
	if maxNum > count {
		maxNum = count
	}
	watchList := WatchList{UserID: uint64(member.User.ID)}

	common.GORM.Model(&watchList).First(&watchList)

	embed := &discordgo.MessageEmbed{
		Title: "Moderation logs",
	}

	if member != nil {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		}
		embed.Description = fmt.Sprintf("**User**: %s (%d)\n", member.User.Mention(), member.User.ID)
	}

	if watchList.HeadModeratorNote != "" {
		embed.Description += fmt.Sprintf("**Head Moderator Note**: %s \n", watchList.HeadModeratorNote)
	}

	embed.Description += "\n>>> "
	for i := 0 + (5 * (page - 1)); i < maxNum; i++ {
		entry := modLogEntries[i]
		embed.Description = embed.Description + fmt.Sprintf("### %s\n **ID**: %d\n **Reason**: %s", entry.Type, entry.ID, entry.Reason)

		if entry.Duration > 0 {
			embed.Description = embed.Description + fmt.Sprintf("\n **Duration**: %s", entry.Duration)
		}

		embed.Description = embed.Description + fmt.Sprintf("\n **Given By**: %s\n **Given At**: <t:%d:f>\n **Log**: %s\n", entry.Author.Mention(), entry.GivenAt.Unix(), fmt.Sprintf("[Link](%s)", entry.LogLink))
	}

	return embed, nil
}

func generatePunishList(guildID int64, userID uint64) ([]ModLogEntry, error) {
	modLogs := ModLog{UserID: userID}
	var modLogEntries []ModLogEntry

	err := common.GORM.Model(&modLogs).Preload("Warns").Preload("Mutes").Preload("Kicks").Preload("Bans").First(&modLogs).Error
	if err != nil {
		log.Warn(err)
		return []ModLogEntry{}, nil
	}

	for _, entry := range modLogs.Warns {
		authorID, parseErr := strconv.ParseInt(entry.AuthorID, 10, 64)
		member, parseErr := bot.GetMember(guildID, authorID)
		if parseErr != nil {
			return nil, err
		}

		modLogEntries = append(modLogEntries, ModLogEntry{Type: "Warn", ID: uint64(entry.ID), Author: member.User, LogLink: entry.LogLink, Reason: entry.Reason, GivenAt: entry.CreatedAt})
	}

	for _, entry := range modLogs.Mutes {
		authorID, parseErr := strconv.ParseInt(entry.AuthorID, 10, 64)
		member, parseErr := bot.GetMember(guildID, authorID)
		if parseErr != nil {
			return nil, err
		}

		modLogEntries = append(modLogEntries, ModLogEntry{Type: "Mute", ID: uint64(entry.ID), Author: member.User, LogLink: entry.LogLink, Reason: entry.Reason, Duration: entry.Duration, GivenAt: entry.CreatedAt})
	}

	for _, entry := range modLogs.Kicks {
		authorID, parseErr := strconv.ParseInt(entry.AuthorID, 10, 64)
		member, parseErr := bot.GetMember(guildID, authorID)
		if parseErr != nil {
			return nil, err
		}

		modLogEntries = append(modLogEntries, ModLogEntry{Type: "Kick", ID: uint64(entry.ID), Author: member.User, LogLink: entry.LogLink, Reason: entry.Reason, GivenAt: entry.CreatedAt})
	}

	for _, entry := range modLogs.Bans {
		authorID, parseErr := strconv.ParseInt(entry.AuthorID, 10, 64)
		member, parseErr := bot.GetMember(guildID, authorID)
		if parseErr != nil {
			return nil, err
		}

		modLogEntries = append(modLogEntries, ModLogEntry{Type: "Ban", ID: uint64(entry.ID), Author: member.User, LogLink: entry.LogLink, Reason: entry.Reason, Duration: entry.Duration, GivenAt: entry.CreatedAt})
	}

	return modLogEntries, nil
}

func generatePunishments(data []ModLogEntry) []*discordgo.MessageEmbedField {
	var punishments []*discordgo.MessageEmbedField

	for i := 0; i < len(data); i++ {
		entry := data[i]
		field := []*discordgo.MessageEmbedField{
			{Name: entry.Type, Value: strconv.FormatUint(entry.ID, 10), Inline: true},
			{Name: "Reason", Value: entry.Reason, Inline: true},
			{Name: "Duration", Value: templates.ToDuration(entry.Duration).String(), Inline: true},
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
		Description: fmt.Sprintf(">>> **User:** %s (%d)\n**Reason:** %s\n **Ping Toggle:** %s", user.Mention(), user.ID, data.Reason, data.Ping),
		Timestamp:   time.Now().Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Watchlist entry last updated by %s", author.Globalname),
			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
		},
	}

	punishList, err := generatePunishList(guildID, uint64(user.ID))
	if err != nil {
		log.Warn("No punishments found")
	} else {
		embed.Fields = append(embed.Fields, generatePunishments(punishList)...)
	}

	if len(data.Feuds) > 0 {
		for _, feud := range data.Feuds {
			fields := []*discordgo.MessageEmbedField{
				{Name: "Feuding User", Value: feud.FeudingUserName, Inline: true},
				{Name: "Reason", Value: feud.Reason, Inline: true},
				{Name: "Link", Value: feud.MessageLink, Inline: true},
			}
			embed.Fields = append(embed.Fields, fields...)
		}
	}

	if len(data.VerbalWarnings) > 0 {
		for _, verbalWarning := range data.VerbalWarnings {
			if verbalWarning.AuthorID != 0 {
				authorMember, memberErr := bot.GetMember(guildID, verbalWarning.AuthorID)
				if memberErr != nil {
					log.Error(memberErr, guildID, verbalWarning)
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
