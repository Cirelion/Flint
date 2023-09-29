package games

import (
	"fmt"
	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/lib/dcmd"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
)

var (
	ResetScrews = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "ResetScrews",
		Description:               "Resets the screws of the given user",
		RequiredArgs:              1,
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		IsResponseEphemeral:       true,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user you want to reset the screws off of", Type: &commands.MemberArg{}},
		},
		RunFunc: resetScrews,
	}
	Screws = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "Screws",
		Description:               "Shows the screws you or the given user has",
		RequiredArgs:              0,
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user you want to see the screws of", Type: &commands.MemberArg{}},
		},
		RunFunc: getScrews,
	}
	GiveScrews = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "GiveScrews",
		Description:               "Gives x amount of screws to another member",
		RequiredArgs:              2,
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to give the screws to", Type: &commands.MemberArg{}},
			{Name: "Screws", Help: "Amount of screws to give", Type: dcmd.Int},
		},
		RunFunc: giveScrews,
	}
)

func getScrews(data *dcmd.Data) (interface{}, error) {
	user := data.Args[0].User()
	player := &Player{}
	mention := data.Author.Mention()

	if user != nil {
		player.UserID = user.ID
		mention = user.Mention()
	} else {
		player.UserID = data.Author.ID
	}
	common.GORM.Model(&player).First(&player)

	if !player.Initialized {
		initPlayer(player)
	}

	return fmt.Sprintf("%s has %d <:tempscrewplschangelater:1156155400509464606> in total!", mention, player.ScrewCount), nil
}
func giveScrews(data *dcmd.Data) (interface{}, error) {
	user := data.Args[0].User()
	screws := data.Args[1].Int64()

	if data.TraditionalTriggerData != nil {
		err := common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
		if err != nil {
			return nil, err
		}
	}

	receivingPlayer := &Player{UserID: user.ID}
	givingPlayer := &Player{UserID: data.Author.ID}
	common.GORM.Model(&receivingPlayer).First(&receivingPlayer)
	common.GORM.Model(&givingPlayer).First(&givingPlayer)

	if !receivingPlayer.Initialized {
		initPlayer(receivingPlayer)

	}

	if !givingPlayer.Initialized {
		initPlayer(givingPlayer)
	}

	if receivingPlayer.UserID == givingPlayer.UserID {
		return "No cheating fuckface", nil
	}

	if givingPlayer.ScrewCount < screws {
		return "You can't give away <:tempscrewplschangelater:1156155400509464606> you don't have dude.", nil
	}

	common.GORM.Model(&givingPlayer).Updates(map[string]interface{}{"screws_given": givingPlayer.ScrewsGiven + screws, "screw_count": givingPlayer.ScrewCount - screws})
	common.GORM.Model(&receivingPlayer).Updates(map[string]interface{}{"screws_received": receivingPlayer.ScrewsReceived + screws, "screw_count": receivingPlayer.ScrewCount + screws})

	return fmt.Sprintf("%s gave %d <:tempscrewplschangelater:1156155400509464606> to %s!", data.Author.Mention(), screws, user.Mention()), nil
}
func resetScrews(data *dcmd.Data) (interface{}, error) {
	user := data.Args[0].User()

	if data.TraditionalTriggerData != nil {
		err := common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
		if err != nil {
			return nil, err
		}
	}

	receivingPlayer := &Player{UserID: user.ID}

	common.GORM.Model(&receivingPlayer).Updates(map[string]interface{}{"screws_given": 0, "screws_received": 0, "screw_count": 50})

	return fmt.Sprintf("%s <:tempscrewplschangelater:1156155400509464606> reset!", user.Mention()), nil
}

func initPlayer(user *Player) {
	user.ScrewCount = 50
	user.Initialized = true

	common.GORM.Model(&user).Save(user)
}
