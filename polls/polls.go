package polls

import (
	"emperror.dev/errors"
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	"github.com/cirelion/flint/moderation"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	urlRegex      = `https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*)`
	pollReactions = [...]string{"1⃣", "2⃣", "3⃣", "4⃣", "5⃣", "6⃣", "7⃣", "8⃣", "9⃣", "🔟"}
	Poll          = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "Poll",
		Description:               "Create regular poll",
		RequiredArgs:              1,
		ApplicationCommandEnabled: true,
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
		CmdCategory:               commands.CategoryTool,
		Name:                      "StrawPoll",
		Description:               "Create a strawpoll",
		RequiredArgs:              3,
		ApplicationCommandEnabled: true,
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
	StartContestRound = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "ContestRound",
		Description:               "Starts a contest between 2 participants",
		RequiredArgs:              4,
		RequireDiscordPerms:       []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp:  "ManageMessages",
		ApplicationCommandEnabled: true,
		Arguments: []*dcmd.ArgDef{
			{
				Name: "Title",
				Type: dcmd.String,
				Help: "The title for the round. (Basic Contest, Round #1: Allie vs Lysandra.)",
			},
			{
				Name: "FirstPost",
				Type: dcmd.BigInt,
				Help: "The ID of the first post for the contest round",
			},
			{
				Name: "SecondPost",
				Type: dcmd.BigInt,
				Help: "The ID of the second post for the contest round",
			},
			{
				Name: "Color",
				Type: dcmd.BigInt,
				Help: "The accent colour of the embed",
			},
		},
		RunFunc: startContestRound,
	}
	EndPoll = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "End Poll",
		RequireDiscordPerms:       []int64{discordgo.PermissionManageMessages},
		RequiredDiscordPermsHelp:  "ManageMessages",
		IsResponseEphemeral:       true,
		ApplicationCommandEnabled: true,
		ApplicationCommandType:    3,
		RunFunc:                   endPoll,
	}
)

func startContestRound(data *dcmd.Data) (interface{}, error) {
	config, err := moderation.GetConfig(data.GuildData.GS.ID)
	if err != nil {
		return nil, err
	}

	roundTitle := data.Args[0].Str()
	firstPost := data.GuildData.GS.GetThread(data.Args[1].Int64())
	firstPostMessages, err := common.BotSession.ChannelMessages(firstPost.ID, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	secondPost := data.GuildData.GS.GetThread(data.Args[2].Int64())
	secondPostMessages, err := common.BotSession.ChannelMessages(secondPost.ID, 0, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	if data.TraditionalTriggerData != nil {
		err = common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
		if err != nil {
			return nil, err
		}
	}

	firstEmbed := generateContestRoundEmbed(firstPost, firstPostMessages[len(firstPostMessages)-1])
	secondEmbed := generateContestRoundEmbed(secondPost, secondPostMessages[len(secondPostMessages)-1])
	pollEmbed := generateContestRoundPollEmbed(roundTitle, firstPost.Name, secondPost.Name, []Vote{}, time.Now().Add(time.Hour*24), false)
	pollEmbed.Color = data.Args[3].Int()

	firstPostCustomID := fmt.Sprintf("contest_round_%s", firstPost.Name)
	secondPostCustomID := fmt.Sprintf("contest_round_%s", secondPost.Name)

	msg, err := common.BotSession.ChannelMessageSendComplex(config.ContestRoundChannel, &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{firstEmbed, secondEmbed, pollEmbed},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    firstPost.Name,
						CustomID: firstPostCustomID,
						Style:    discordgo.PrimaryButton,
					},
					discordgo.Button{
						Label:    secondPost.Name,
						CustomID: secondPostCustomID,
						Style:    discordgo.PrimaryButton,
					},
				},
			},
		},
	})

	if err != nil {
		return nil, errors.WrapIf(err, "failed to start contest round")
	}

	contestRound := &ContestRound{MessageID: msg.ID, ChannelID: msg.ChannelID, GuildID: data.GuildData.GS.ID, FirstPost: firstPost.Name, SecondPost: secondPost.Name}
	common.GORM.Model(contestRound).Save(contestRound)
	go contestRound.handleContestTimer()
	return "## May the best bot win.", nil
}

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

	_, err := CreatePollEmbed(data.Session, data.SlashCommandTriggerData.Interaction.Token, data.GuildData.GS.ID, data.Author, question, votes)

	if err != nil {
		return nil, errors.WrapIf(err, "failed to add poll")
	}

	return nil, nil
}

func createStrawPoll(data *dcmd.Data) (interface{}, error) {
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

	_, err := CreateStrawPollEmbed(data.Session, data.SlashCommandTriggerData.Interaction.Token, data.GuildData.GS.ID, data.Author, question, selectMenuOptions, maxOptions)
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

func createSelectMenu(options []SelectMenuOption, maxOptions int) []discordgo.MessageComponent {
	placeholder := "Select an option to vote for."

	if maxOptions > 1 {
		placeholder = fmt.Sprintf("Select up to %d options to vote for.", maxOptions)
	}
	var selectOptions []discordgo.SelectMenuOption

	for _, option := range options {
		selectOptions = append(selectOptions, discordgo.SelectMenuOption{
			Label: option.Label,
			Value: option.Value,
			Emoji: discordgo.ComponentEmoji{
				Name: option.EmojiName,
			},
		})
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    "straw_poll_select_menu",
					Placeholder: placeholder,
					Options:     selectOptions,
					MaxValues:   maxOptions,
				},
			},
		},
	}
}

func CreatePollEmbed(session *discordgo.Session, token string, guildID int64, author *discordgo.User, question string, votes []Vote) (*PollMessage, error) {
	pm := &PollMessage{
		GuildID:  guildID,
		AuthorID: author.ID,
		Question: question,
		Votes:    votes,
	}

	embed, err := PollEmbed(question, author, votes)
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
	err = common.GORM.Model(&pm).Save(&pm).Error
	if err != nil {
		return nil, err
	}

	return pm, nil
}

func CreateStrawPollEmbed(session *discordgo.Session, token string, guildID int64, author *discordgo.User, question string, options []SelectMenuOption, maxOptions int) (*PollMessage, error) {
	pm := &PollMessage{
		GuildID:     guildID,
		AuthorID:    author.ID,
		Question:    question,
		IsStrawPoll: true,
		Options:     options,
		MaxOptions:  maxOptions,
	}

	embed, err := StrawPollEmbed(question, options, author, nil)
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
	err = common.GORM.Model(&pm).Save(&pm).Error

	if err != nil {
		return nil, err
	}

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
		parsedVotes := strings.Split(value.Vote, ", ")
		if parsedVotes[0] == "0" {
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
		Description: fmt.Sprintf("<:Check:1247310020102721688> `%d %s (%d%s)` <:Cross:1247309988129669312> `%d %s (%d%s)`", yay, yayVoteString, int(yayPercentage), "%", nay, nayVoteString, int(nayPercentage), "%"),
		Footer: &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Asked by %s", author.Globalname),
			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
		},
	}

	return &embed, nil
}

func StrawPollEmbed(question string, options []SelectMenuOption, author *discordgo.User, votes []Vote) (*discordgo.MessageEmbed, error) {
	var description string

	for i, option := range options {
		voteString := "votes"
		totalCount := 0
		voteCount := 0
		votePercentage := float64(0)

		for _, value := range votes {
			parsedVotes := strings.Split(value.Vote, ", ")
			totalCount += len(parsedVotes)
			for _, parsedVote := range parsedVotes {
				voteNum, _ := strconv.Atoi(parsedVote)
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

func generateContestRoundPollEmbed(roundTitle string, firstPost string, secondPost string, votes []Vote, timeStamp time.Time, ended bool) *discordgo.MessageEmbed {
	totalVoteCount := len(votes)
	firstVoteCount := 0
	secondVoteCount := 0
	firstVotePercentage := float64(0)
	secondVotePercentage := float64(0)

	if totalVoteCount > 0 {
		for _, value := range votes {
			customID := fmt.Sprintf("contest_round_%s", firstPost)
			if value.Vote == customID {
				firstVoteCount++
			} else {
				secondVoteCount++
			}
		}

		if firstVoteCount > 0 {
			firstVotePercentage = (float64(firstVoteCount) / float64(totalVoteCount)) * 100
		}

		if secondVoteCount > 0 {
			secondVotePercentage = (float64(secondVoteCount) / float64(totalVoteCount)) * 100
		}
	}

	description := fmt.Sprintf("**%s**: %d (%d%s)\n**%s**: %d (%d%s)", firstPost, firstVoteCount, int(firstVotePercentage), "%", secondPost, secondVoteCount, int(secondVotePercentage), "%")
	footerText := "Round will end"

	if ended {
		description += "\n\n*Vote has ended*"
		footerText = "Round ended"
	} else {
		description += "\n\n*Vote for your* favourite character below!"
	}

	return &discordgo.MessageEmbed{
		Title:       roundTitle,
		Description: description,
		Timestamp:   timeStamp.Format(time.RFC3339),
		Footer: &discordgo.MessageEmbedFooter{
			Text: footerText,
		},
	}
}

func generateContestRoundEmbed(channel *dstate.ChannelState, message *discordgo.Message) *discordgo.MessageEmbed {
	member, err := bot.GetMember(channel.GuildID, channel.OwnerID)
	if err != nil {
		logger.Error(err)
	}

	embed := &discordgo.MessageEmbed{
		Title:       channel.Name,
		Description: message.Content,
		Footer: &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Created by %s", bot.GetName(member)),
			IconURL: discordgo.EndpointUserAvatar(member.User.ID, member.User.Avatar),
		},
	}

	re, _ := regexp.Compile(urlRegex)
	if re.MatchString(message.Content) {
		embed.URL = re.FindString(message.Content)
		embed.Description = strings.Replace(message.Content, embed.URL, "", -1)
	}

	if len(message.Attachments) > 0 {
		embed.Image = &discordgo.MessageEmbedImage{
			URL:      message.Attachments[0].URL,
			ProxyURL: message.Attachments[0].ProxyURL,
			Width:    message.Attachments[0].Width,
			Height:   message.Attachments[0].Height,
		}
	}

	return embed
}

func generateSelectMenuOptions(options []*dcmd.ParsedArg) []SelectMenuOption {
	var selectMenuOptions []SelectMenuOption
	for i, option := range options {
		if option.Str() == "" || i >= len(pollReactions) {
			options = options[:i]
			break
		}
	}

	for i, option := range options {
		selectMenuOptions = append(selectMenuOptions, SelectMenuOption{
			Label:     option.Str(),
			Value:     strconv.Itoa(i),
			EmojiName: pollReactions[i],
		})
	}

	return selectMenuOptions
}
