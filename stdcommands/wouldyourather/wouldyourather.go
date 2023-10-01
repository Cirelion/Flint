package wouldyourather

import (
	"context"
	"fmt"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/fun"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
	"math/rand"
	"strings"
)

type WouldYouRather struct {
	OptionA string
	OptionB string
}

func randomQuestion(questionString string) *WouldYouRather {
	questions := strings.Split(questionString, "\n")
	question := strings.TrimSpace(questions[rand.Intn(len(questions))])
	wyr := strings.Split(question, " - ")

	if len(wyr) > 1 {
		return &WouldYouRather{
			OptionA: strings.TrimSpace(wyr[0]),
			OptionB: strings.TrimSpace(wyr[1]),
		}
	}

	return nil
}

func handleWyr(data *dcmd.Data, question *WouldYouRather) error {
	wyrDescription := fmt.Sprintf("**EITHER...**\n🇦 %s\n\n **OR...**\n🇧 %s", question.OptionA, question.OptionB)

	embed := &discordgo.MessageEmbed{
		Description: wyrDescription,
		Author: &discordgo.MessageEmbedAuthor{
			Name: "Would you rather...",
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("Requested by: %s", data.Author.Globalname),
		},
		Color: rand.Intn(16777215),
	}

	msg, err := common.BotSession.ChannelMessageSendEmbed(data.ChannelID, embed)
	if err != nil {
		return err
	}

	common.BotSession.MessageReactionAdd(data.ChannelID, msg.ID, "🇦")
	err = common.BotSession.MessageReactionAdd(data.ChannelID, msg.ID, "🇧")
	if err != nil {
		return err
	}

	return nil
}

var Wyr = &commands.YAGCommand{
	CmdCategory:               commands.CategoryFun,
	Name:                      "WouldYouRather",
	Aliases:                   []string{"wyr"},
	Description:               "Get presented with 2 options.",
	ApplicationCommandEnabled: true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		config, err := fun.GetConfig(context.Background(), data.GuildData.GS.ID)
		if err != nil {
			return false, err
		}

		question := randomQuestion(config.Wyrs)
		if question == nil {
			return "No WYR found", nil
		}

		err = handleWyr(data, question)
		return nil, err
	},
}

var NSFWWyr = &commands.YAGCommand{
	CmdCategory:               commands.CategoryFun,
	Name:                      "NSFWWouldYouRather",
	Aliases:                   []string{"nsfwwyr"},
	Description:               "Get presented with 2 options.",
	ApplicationCommandEnabled: true,
	NSFW:                      true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		config, err := fun.GetConfig(context.Background(), data.GuildData.GS.ID)
		if err != nil {
			return false, err
		}

		question := randomQuestion(config.NSFWWyrs)
		if question == nil {
			return "No WYR found", nil
		}

		err = handleWyr(data, question)
		return nil, err
	},
}
