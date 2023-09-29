package heartboard

import (
	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/lib/dcmd"
	"log"
)

var (
	AddToHeartBoard = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "AddToHeartBoard",
		Description:               "Adds post to heart board",
		RequiredArgs:              1,
		ApplicationCommandEnabled: true,
		Arguments: []*dcmd.ArgDef{
			{
				Name: "MessageID",
				Type: dcmd.Int,
				Help: "The MessageID of the post you want to add to the heartboard",
			},
		},
		RunFunc: addToHeartBoard,
	}
)

func addToHeartBoard(data *dcmd.Data) (interface{}, error) {
	log.Println(data)

	if data.TraditionalTriggerData != nil {
		err := common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}

//func PollEmbed(question string, author *discordgo.User, votes []Vote) (*discordgo.MessageEmbed, error) {
//	count := len(votes)
//	yay := 0
//	nay := 0
//	nayVoteString := "votes"
//	yayVoteString := "votes"
//	yayPercentage := float64(0)
//	nayPercentage := float64(0)
//
//	for _, value := range votes {
//		parsedVotes := strings.Split(value.Vote, ", ")
//		if parsedVotes[0] == "0" {
//			yay++
//		} else {
//			nay++
//		}
//	}
//
//	if count > 0 {
//		yayPercentage = (float64(yay) / float64(count)) * 100
//		nayPercentage = (float64(nay) / float64(count)) * 100
//
//		if yay == 1 {
//			yayVoteString = "vote"
//		}
//
//		if nay == 1 {
//			nayVoteString = "vote"
//		}
//	}
//
//	embed := discordgo.MessageEmbed{
//		Title:       question,
//		Color:       0x65f442,
//		Description: fmt.Sprintf("<:Check:1153636410771906630> `%d %s (%d%s)` <:Cross:1153636407714271252> `%d %s (%d%s)`", yay, yayVoteString, int(yayPercentage), "%", nay, nayVoteString, int(nayPercentage), "%"),
//		Footer: &discordgo.MessageEmbedFooter{
//			Text:    fmt.Sprintf("Asked by %s", author.Globalname),
//			IconURL: discordgo.EndpointUserAvatar(author.ID, author.Avatar),
//		},
//	}
//
//	return &embed, nil
//}
