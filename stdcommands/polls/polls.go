package polls

import (
	"emperror.dev/errors"
	"fmt"
	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/lib/dcmd"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
	"strconv"
	"strings"
	"time"
)

var (
	pollReactions = [...]string{"1⃣", "2⃣", "3⃣", "4⃣", "5⃣", "6⃣", "7⃣", "8⃣", "9⃣", "🔟"}
	Poll          = &commands.YAGCommand{
		CmdCategory:         commands.CategoryTool,
		Name:                "Poll",
		Description:         "Create regular poll",
		RequiredArgs:        1,
		SlashCommandEnabled: true,
		Arguments: []*dcmd.ArgDef{
			{
				Name: "Question",
				Type: dcmd.String,
				Help: "The question you want to ask",
			},
		},
		RunFunc: createPoll,
	}
	StrawPoll = &commands.YAGCommand{
		CmdCategory:         commands.CategoryTool,
		Name:                "StrawPoll",
		Description:         "Create a strawpoll",
		RequiredArgs:        3,
		SlashCommandEnabled: true,
		Arguments: []*dcmd.ArgDef{
			{
				Name: "Question",
				Type: dcmd.String,
				Help: "The question you want to ask",
			},
			{Name: "Option-1", Type: dcmd.String},
			{Name: "Option-2", Type: dcmd.String},
			{Name: "Option-3", Type: dcmd.String},
			{Name: "Option-4", Type: dcmd.String},
			{Name: "Option-5", Type: dcmd.String},
			{Name: "Option-6", Type: dcmd.String},
			{Name: "Option-7", Type: dcmd.String},
			{Name: "Option-8", Type: dcmd.String},
			{Name: "Option-9", Type: dcmd.String},
			{Name: "Option-10", Type: dcmd.String},
		},
		ArgSwitches: []*dcmd.ArgDef{
			{Name: "maxOptions", Help: "Maximum number of options the member can select", Type: &dcmd.IntArg{Max: 10}, Default: 1},
		},
		RunFunc: createStrawPoll,
	}
	EndPoll = &commands.YAGCommand{
		CmdCategory:              commands.CategoryTool,
		Name:                     "EndPoll",
		RequireDiscordPerms:      []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp: "ManageMessages",
		IsResponseEphemeral:      true,
		SlashCommandEnabled:      true,
		ContextMenuMessage:       true,
		RunFunc:                  endPoll,
	}
)

func endPoll(data *dcmd.Data) (interface{}, error) {
	channelID := data.ChannelID
	messageID := data.SlashCommandTriggerData.Interaction.DataCommand.TargetID

	msg, err := common.BotSession.ChannelMessage(channelID, messageID)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to get message")
	}

	if len(msg.Embeds) > 0 {
		embed := msg.Embeds[0]

		if strings.Contains(embed.Footer.Text, "Asked by") {
			embed.Description = fmt.Sprintf("Poll ended <t:%d:R>\n\n", time.Now().Unix()) + embed.Description
			msg, err = common.BotSession.ChannelMessageEditComplex(&discordgo.MessageEdit{
				Embeds:     []*discordgo.MessageEmbed{embed},
				Components: []discordgo.MessageComponent{},
				Channel:    channelID,
				ID:         messageID,
			})

			if err != nil {
				return nil, errors.WrapIf(err, "failed to end poll")
			}

			return "Done", nil
		}
	}
	return "Not a poll.", nil
}

func createPoll(data *dcmd.Data) (interface{}, error) {
	var votes []Vote
	question := data.Args[0].Str()

	if data.TraditionalTriggerData != nil {
		err := common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
		if err != nil {
			return nil, err
		}
	}

	_, err := CreatePollEmbed(data.Session, data.SlashCommandTriggerData.Interaction.Token, data.GuildData.GS.ID, data.ChannelID, votes, func(p *PollMessage, votes []Vote) (*discordgo.MessageEmbed, error) {
		return PollEmbed(question, data.Author, votes)
	})

	if err != nil {
		return nil, errors.WrapIf(err, "failed to add poll")
	}

	return nil, nil
}

func createStrawPoll(data *dcmd.Data) (interface{}, error) {
	var votes []Vote
	question := data.Args[0].Str()
	options := data.Args[1:]
	maxOptions := data.Switch("maxOptions").Int()

	if data.TraditionalTriggerData != nil {
		err := common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
		if err != nil {
			return nil, err
		}
	}

	selectMenuOptions := generateSelectMenuOptions(options)

	_, err := CreateStrawPollEmbed(data.Session, data.SlashCommandTriggerData.Interaction.Token, data.GuildData.GS.ID, data.ChannelID, votes, selectMenuOptions, maxOptions, func(p *PollMessage, votes []Vote) (*discordgo.MessageEmbed, error) {
		return StrawPollEmbed(question, options, data.Author, votes)
	})

	if err != nil {
		return nil, errors.WrapIf(err, "failed to add straw poll")
	}

	return nil, nil
}

func createYesOrNoButtons() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: "poll_yay",
					Style:    3,
					Emoji: discordgo.ComponentEmoji{
						ID:   1153636410771906630,
						Name: "Check",
					},
				},
				discordgo.Button{
					CustomID: "poll_nay",
					Style:    4,
					Emoji: discordgo.ComponentEmoji{
						ID:   1153636407714271252,
						Name: "Cross",
					},
				},
			},
		},
	}
}

func createSelectMenu(options []discordgo.SelectMenuOption, maxOptions int) []discordgo.MessageComponent {
	placeholder := "Select an option to vote for."

	if maxOptions > 1 {
		placeholder = fmt.Sprintf("Select up to %d options to vote for.", maxOptions)
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    "strawpoll_selectmenu",
					Placeholder: placeholder,
					Options:     options,
					MaxValues:   maxOptions,
				},
			},
		},
	}
}

func CreatePollEmbed(session *discordgo.Session, token string, guildID int64, channelID int64, votes []Vote, pollFunc PollFunc) (*PollMessage, error) {
	pm := &PollMessage{
		GuildID:        guildID,
		ChannelID:      channelID,
		Votes:          votes,
		lastUpdateTime: time.Now(),
		stopCh:         make(chan bool),
		HandleVote:     pollFunc,
	}

	embed, err := pollFunc(pm, votes)
	if err != nil {
		return nil, err
	}
	embed.Timestamp = time.Now().Format(time.RFC3339)

	msg, err := session.EditOriginalInteractionResponse(common.BotApplication.ID, token, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: createYesOrNoButtons(),
	})

	if err != nil {
		return nil, err
	}

	pm.MessageID = msg.ID
	pm.LastResponse = embed

	menusLock.Lock()
	activePollMessagesMap[pm.MessageID] = pm
	menusLock.Unlock()

	return pm, nil
}

func CreateStrawPollEmbed(session *discordgo.Session, token string, guildID int64, channelID int64, votes []Vote, options []discordgo.SelectMenuOption, maxOptions int, pollFunc PollFunc) (*PollMessage, error) {
	pm := &PollMessage{
		GuildID:        guildID,
		ChannelID:      channelID,
		Votes:          votes,
		lastUpdateTime: time.Now(),
		stopCh:         make(chan bool),
		HandleVote:     pollFunc,
	}

	embed, err := pollFunc(pm, votes)
	if err != nil {
		return nil, err
	}
	embed.Timestamp = time.Now().Format(time.RFC3339)

	msg, err := session.EditOriginalInteractionResponse(common.BotApplication.ID, token, &discordgo.WebhookParams{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: createSelectMenu(options, maxOptions),
	})

	if err != nil {
		return nil, err
	}

	pm.MessageID = msg.ID
	pm.LastResponse = embed

	menusLock.Lock()
	activePollMessagesMap[pm.MessageID] = pm
	menusLock.Unlock()

	return pm, nil
}

func PollEmbed(question string, author *discordgo.User, votes []Vote) (*discordgo.MessageEmbed, error) {
	count := len(votes)
	yay := 0
	nay := 0
	nayVoteString := "votes"
	yayVoteString := "votes"
	yayPercentage := float64(0)
	nayPercentage := float64(0)

	for _, value := range votes {
		if value.vote[0] == 0 {
			yay++
		} else {
			nay++
		}
	}

	if count > 0 {
		yayPercentage = (float64(yay) / float64(count)) * 100
		nayPercentage = (float64(nay) / float64(count)) * 100

		if yay == 1 {
			yayVoteString = "vote"
		}

		if nay == 1 {
			nayVoteString = "vote"
		}
	}

	embed := discordgo.MessageEmbed{
		Title:       question,
		Color:       0x65f442,
		Description: fmt.Sprintf("<:Check:1153636410771906630> `%d %s (%d%s)` <:Cross:1153636407714271252> `%d %s (%d%s)`", yay, yayVoteString, int(yayPercentage), "%", nay, nayVoteString, int(nayPercentage), "%"),
		Footer: &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Asked by %s", author.Globalname),
			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
		},
	}

	return &embed, nil
}

func StrawPollEmbed(question string, options []*dcmd.ParsedArg, author *discordgo.User, votes []Vote) (*discordgo.MessageEmbed, error) {
	selectMenuOptions := generateSelectMenuOptions(options)

	var description string
	for i, option := range selectMenuOptions {
		voteString := "votes"
		totalCount := 0
		voteCount := 0
		votePercentage := float64(0)

		for _, value := range votes {
			totalCount += len(value.vote)
			for _, voteNum := range value.vote {
				if voteNum == i {
					voteCount++
				}
			}
		}

		if totalCount > 0 {
			votePercentage = float64(voteCount) / float64(totalCount) * 100

			if voteCount == 1 {
				voteString = "vote"
			}
		}

		description += fmt.Sprintf("%s - %s\n`%d %s (%d%s)`\n", pollReactions[i], option.Label, voteCount, voteString, int(votePercentage), "%")
	}

	embed := discordgo.MessageEmbed{
		Title:       question,
		Description: description,
		Color:       0x65f442,
		Footer: &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Asked by %s", author.Globalname),
			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
		},
	}

	return &embed, nil
}

func generateSelectMenuOptions(options []*dcmd.ParsedArg) []discordgo.SelectMenuOption {
	var selectMenuOptions []discordgo.SelectMenuOption
	for i, option := range options {
		if option.Str() == "" || i >= len(pollReactions) {
			options = options[:i]
			break
		}
	}

	for i, option := range options {
		selectMenuOptions = append(selectMenuOptions, discordgo.SelectMenuOption{
			Label: option.Str(),
			Value: strconv.Itoa(i),
			Emoji: discordgo.ComponentEmoji{
				Name: pollReactions[i],
			},
		})
	}

	return selectMenuOptions
}
