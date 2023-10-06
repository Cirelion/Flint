package giveaways

import (
	"fmt"
	"github.com/AlekSi/pointer"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	"math/rand"
	"time"
)

var (
	StartGiveaway = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "Giveaway",
		Description:               "Starts a giveaway",
		RequiredArgs:              2,
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		RequireDiscordPerms:       []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp:  "ManageMessages",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		IsResponseEphemeral:       true,
		Arguments: []*dcmd.ArgDef{
			{Name: "Prize", Help: "The prize of the giveaway", Type: dcmd.String},
			{Name: "Duration", Help: "The duration of the giveaway", Type: &commands.DurationArg{}},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "MaxWinners", Help: "Maximum amount of winners", Type: dcmd.Int, Default: 1},
			{Name: "Channel", Help: "The channel to post the giveaway in", Type: dcmd.Channel},
			{Name: "Co-host", Help: "Potential co-host for the giveaway", Type: &commands.MemberArg{}},
		},
		RunFunc: startGiveaway,
	}
	CancelGiveaway = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "Cancel Giveaway",
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		RequireDiscordPerms:       []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp:  "ManageMessages",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		IsResponseEphemeral:       true,
		ApplicationCommandType:    3,
		RunFunc:                   cancelGiveaway,
	}
	RerollGiveaway = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "Reroll Giveaway",
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		RequireDiscordPerms:       []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp:  "ManageMessages",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		IsResponseEphemeral:       false,
		ApplicationCommandType:    3,
		RunFunc:                   rerollGiveaway,
	}
)

func rerollGiveaway(data *dcmd.Data) (interface{}, error) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	messageID := data.SlashCommandTriggerData.Interaction.DataCommand.TargetID
	giveaway := Giveaway{
		MessageID: messageID,
	}
	err := common.GORM.Model(&giveaway).First(&giveaway).Error
	if err != nil {
		return "Not a giveaway", nil
	}
	randomIndex := r.Intn(len(giveaway.Participants))
	member, err := bot.GetMember(giveaway.GuildID, giveaway.Participants[randomIndex])
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Giveaway reroll result: %s", member.User.Mention()), nil
}

func endGiveaway(data *dcmd.Data) (interface{}, error) {
	messageID := data.SlashCommandTriggerData.Interaction.DataCommand.TargetID
	giveaway := Giveaway{
		MessageID: messageID,
	}
	err := common.GORM.Model(&giveaway).First(&giveaway).Error
	if err != nil {
		return "Not a giveaway", nil
	}

	return handleEndGiveaway(giveaway)
}

func cancelGiveaway(data *dcmd.Data) (interface{}, error) {
	messageID := data.SlashCommandTriggerData.Interaction.DataCommand.TargetID
	giveaway := Giveaway{
		MessageID: messageID,
	}

	err := common.GORM.Model(&giveaway).First(&giveaway).Error
	if err != nil {
		return "Not a giveaway", nil
	}

	if !pointer.GetBool(giveaway.Active) {
		return "Giveaway is already inactive", nil
	}

	msg, err := common.BotSession.ChannelMessage(giveaway.ChannelID, giveaway.MessageID)

	if err != nil {
		return "Message doesn't exist anymore", nil
	}

	embed := msg.Embeds[0]
	embed.Timestamp = ""
	embed.Title = "Giveaway has been cancelled"
	embed.Color = 16763170
	embed.Footer = &discordgo.MessageEmbedFooter{Text: "Giveaway cancelled"}
	common.BotSession.ChannelMessageEditEmbed(giveaway.ChannelID, giveaway.MessageID, embed)

	common.GORM.Model(&giveaway).Updates([]interface{}{Giveaway{Active: common.BoolToPointer(false)}})

	return "Giveaway canceled", nil
}

func startGiveaway(data *dcmd.Data) (interface{}, error) {
	prize := data.Args[0].Str()
	duration := data.Args[1].Value.(time.Duration)
	maxWinners := data.Switch("MaxWinners").Int64()
	channelID := data.ChannelID

	giveaway := Giveaway{
		Active:     common.BoolToPointer(true),
		UserID:     data.Author.ID,
		GuildID:    data.GuildData.GS.ID,
		MaxWinners: maxWinners,
		Prize:      prize,
		EndsAt:     time.Now().Add(duration),
	}

	if data.Switch("Channel").Value != nil {
		channelID = data.Switch("Channel").Value.(*dstate.ChannelState).ID
	}

	if data.Switch("Co-host").Value != nil {
		giveaway.UserID = data.Switch("Co-host").User().ID
	}

	giveaway.ChannelID = channelID
	embed, err := generateGiveawayEmbed(giveaway)
	if err != nil {
		return nil, err
	}

	message, err := common.BotSession.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		return nil, err
	}

	err = common.BotSession.MessageReactionAdd(channelID, message.ID, "🎉")
	if err != nil {
		return nil, err
	}

	giveaway.MessageID = message.ID

	common.GORM.Model(&giveaway).Save(&giveaway)
	go giveawayTracker(giveaway)

	return fmt.Sprintf("Giveaway for %s started! Giveaway will end on <t:%d:f>", prize, giveaway.EndsAt.Unix()), nil
}

func generateGiveawayDescription(giveaway Giveaway) (string, error) {
	winnerCount := len(giveaway.Winners)
	member, err := bot.GetMember(giveaway.GuildID, giveaway.UserID)
	if err != nil {
		return "", err
	}
	description := fmt.Sprintf(">>> **Prize**: %s\n\n", giveaway.Prize)

	if giveaway.MaxWinners > 1 && winnerCount == 0 {
		description += fmt.Sprintf("**Max winners**: %d\n\n", giveaway.MaxWinners)
	}

	if winnerCount > 0 {
		if winnerCount > 1 {
			description += "**Winners**: "
		} else {
			description += "**Winner**: "
		}

		for _, winner := range giveaway.Winners {
			winnerMember, memberErr := bot.GetMember(giveaway.GuildID, winner)
			if memberErr != nil {
				continue
			}

			description += fmt.Sprintf("%s, ", winnerMember.User.Mention())
		}
	}

	description = fmt.Sprintf("%s\n\n**Hosted By**: %s\n\n", description[:len(description)-2], member.User.Mention())

	if winnerCount == 0 {
		description += "React with 🎉 to enter the giveaway"
	}

	return description, nil
}

func generateGiveawayEmbed(giveaway Giveaway) (*discordgo.MessageEmbed, error) {
	description, err := generateGiveawayDescription(giveaway)
	if err != nil {
		return nil, err
	}
	embed := discordgo.MessageEmbed{
		Title:       "Giveaway started!",
		Description: description,
		Timestamp:   giveaway.EndsAt.Format(time.RFC3339),
		Color:       4225618,
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Giveaway will end",
		},
	}

	return &embed, nil
}

func removeUser(slice []*discordgo.User, s int) []*discordgo.User {
	return append(slice[:s], slice[s+1:]...)
}
