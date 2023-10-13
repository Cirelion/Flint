package games

import (
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
	log "github.com/sirupsen/logrus"
	"math/rand"
	"time"
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
	InitDuel = &commands.YAGCommand{
		CmdCategory:    commands.CategoryTool,
		Name:           "Duel",
		Aliases:        []string{"challenge"},
		Description:    "Challenges the other user to a duel",
		RequiredArgs:   1,
		DefaultEnabled: true,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to challenge", Type: &commands.MemberArg{}},
			{Name: "Screws", Help: "Amount of screws to bet", Type: dcmd.Int},
		},
		RunFunc: initDuel,
	}
	AcceptDuel = &commands.YAGCommand{
		CmdCategory:    commands.CategoryTool,
		Name:           "AcceptDuel",
		Aliases:        []string{"acceptchallenge"},
		Description:    "Accepts the duel",
		DefaultEnabled: true,
		RunFunc:        acceptDuel,
		RequiredArgs:   0,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user to challenge", Type: &commands.MemberArg{}},
		},
	}
	Fire = &commands.YAGCommand{
		CmdCategory:    commands.CategoryTool,
		Name:           "Shoot",
		Aliases:        []string{"fire"},
		Description:    "Shoots your opponent",
		DefaultEnabled: true,
		RunFunc:        fire,
	}
)

func fire(data *dcmd.Data) (interface{}, error) {
	duel := &Duel{}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	percentage := 1 + r.Intn(10-1)
	randomIndex := r.Intn(len(misses))

	err := common.GORM.Model(Duel{ChallengedID: data.Author.ID}).Where("duel_state != ?", DuelEnded).Last(duel, "challenged_id = ?", data.Author.ID).Error
	if err != nil {
		err = common.GORM.Model(Duel{ChallengerID: data.Author.ID}).Where("duel_state != ?", DuelEnded).Last(duel, "challenger_id = ?", data.Author.ID).Error
		if err != nil {
			return nil, nil
		}
	}

	if duel.DuelState < DuelAccepted {
		return "Don't cheat mf.", nil
	}

	if duel.DuelState < DuelActive {
		return fmt.Sprintf("Shot too early, %s missed!", data.Author.Mention()), nil
	}

	winningPlayer := &Player{UserID: data.Author.ID}
	losingPlayer := &Player{UserID: duel.ChallengerID}
	if winningPlayer.UserID == duel.ChallengerID {
		losingPlayer.UserID = duel.ChallengedID
	}
	winningMember, _ := bot.GetMember(data.GuildData.GS.ID, data.Author.ID)
	losingMember, _ := bot.GetMember(duel.GuildID, losingPlayer.UserID)

	if percentage <= 6 {
		duel.WinnerID = data.Author.ID
		duel.DuelState = DuelEnded

		err = common.GORM.Model(duel).Update(duel).Error
		common.GORM.Model(&winningPlayer).First(&winningPlayer)
		common.GORM.Model(&losingPlayer).First(&losingPlayer)

		if duel.Bet > 0 {
			common.GORM.Model(&winningPlayer).Updates(map[string]interface{}{"screws_received": winningPlayer.ScrewsReceived + duel.Bet, "screw_count": winningPlayer.ScrewCount + duel.Bet})
			common.GORM.Model(&losingPlayer).Updates(map[string]interface{}{"screws_given": losingPlayer.ScrewsGiven + duel.Bet, "screw_count": losingPlayer.ScrewCount - duel.Bet})

			return fmt.Sprintf("Shot %s and took %d <:tempscrewplschangelater:1156155400509464606>", losingMember.User.Mention(), duel.Bet), nil
		}

		return fmt.Sprintf("%s, %s (hit)", winningMember.User.Mention(), wins[randomIndex]), nil
	}

	return fmt.Sprintf("%s, %s (miss)", winningMember.User.Mention(), misses[randomIndex]), nil
}

func handleDuelTimer(channelID int64, duel *Duel) {
	ticker := time.NewTicker(time.Second)
	timer := 10

	challengingMember, err := bot.GetMember(duel.GuildID, duel.ChallengerID)
	challengedMember, err := bot.GetMember(duel.GuildID, duel.ChallengedID)
	if err != nil {
		return
	}

	msg, err := common.BotSession.ChannelMessageSend(channelID, fmt.Sprintf("Duel has started between %s and %s! Participants, prepare your arms!", challengingMember.User.Mention(), challengedMember.User.Mention()))

	if err != nil {
		log.Error(err)
		return
	}

	time.Sleep(time.Second * 3)

	for {
		<-ticker.C
		if timer > 0 {
			common.BotSession.ChannelMessageEdit(channelID, msg.ID, fmt.Sprintf("%d...", timer))
			timer--
		} else {
			duel.DuelState = DuelActive
			err = common.GORM.Model(duel).Update(duel).Error
			if err != nil {
				log.Error(err)
				ticker.Stop()
				return
			}

			common.BotSession.ChannelMessageEdit(channelID, msg.ID, "-fire!")
			ticker.Stop()
		}
	}
}

func acceptDuel(data *dcmd.Data) (interface{}, error) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	user := data.Args[0].User()
	canSteal := r.Intn(3) == 2
	duel := &Duel{}

	if user != nil {
		if user.ID == data.Author.ID {
			return "You can't accept your own duel.", nil
		}

		duel.ChallengerID = user.ID
	} else {
		duel.ChallengedID = data.Author.ID
	}

	err := common.GORM.Model(duel).Last(duel).Error
	if err != nil {
		return nil, err
	}

	if duel.ChallengedID != data.Author.ID {
		if !canSteal {
			return fmt.Sprintf("%s, You aren't being challenged mf", data.Author.Mention()), nil
		}
	}

	if duel.DuelState != DuelInactive {
		return "Duel is already accepted.", nil
	}

	err = common.GORM.Model(duel).Update("duel_state", DuelAccepted).Error
	if err != nil {
		return nil, err
	}

	if time.Since(duel.CreatedAt) > time.Minute {
		return fmt.Sprintf("Duel acceptance duration has expired."), nil
	}

	go handleDuelTimer(data.ChannelID, duel)

	if duel.ChallengedID != data.Author.ID {
		member, _ := bot.GetMember(duel.GuildID, duel.ChallengedID)
		err = common.GORM.Model(duel).Update("challenged_id", data.Author.ID).Error

		return fmt.Sprintf("%s has stolen %s's challenge from %s! Use -fire after I say!", data.Author.Mention(), user.Mention(), member.User.Mention()), nil
	} else {
		member, _ := bot.GetMember(duel.GuildID, duel.ChallengerID)
		return fmt.Sprintf("%s has accepted %s's challenge! Use -fire after I say!", data.Author.Mention(), member.User.Mention()), nil
	}
}

func initDuel(data *dcmd.Data) (interface{}, error) {
	user := data.Args[0].User()
	screws := data.Args[1].Int64()

	challengedPlayer := &Player{UserID: user.ID}
	common.GORM.Model(&challengedPlayer).First(&challengedPlayer)
	if !challengedPlayer.Initialized {
		initPlayer(challengedPlayer)
	}

	challengingPlayer := &Player{UserID: data.Author.ID}
	common.GORM.Model(&challengingPlayer).First(&challengingPlayer)

	if !challengingPlayer.Initialized {
		initPlayer(challengingPlayer)
	}

	if challengingPlayer.UserID == challengedPlayer.UserID {
		return "You can't duel yourself silly.", nil
	}

	if challengingPlayer.ScrewCount < screws {
		return fmt.Sprintf("You can't bet <:tempscrewplschangelater:1156155400509464606> you don't have %s.", data.Author.Mention()), nil
	}

	if challengedPlayer.ScrewCount < screws {
		return fmt.Sprintf("%s doesn't have enough <:tempscrewplschangelater:1156155400509464606>.", user.Mention()), nil
	}

	duel := &Duel{
		ChallengerID: challengingPlayer.UserID,
		ChallengedID: challengedPlayer.UserID,
		GuildID:      data.GuildData.GS.ID,
		Bet:          screws,
		CreatedAt:    time.Time{},
		UpdatedAt:    time.Time{},
	}

	common.GORM.Model(duel).Save(duel)

	if screws > 0 {
		return fmt.Sprintf("%s has challenged %s to a duel for %d <:tempscrewplschangelater:1156155400509464606>! use -acceptduel %s to start the duel!", data.Author.Mention(), user.Mention(), screws, data.Author.Mention()), nil
	} else {
		return fmt.Sprintf("%s has challenged %s to a duel! use -acceptduel %s to start the duel!", data.Author.Mention(), user.Mention(), data.Author.Mention()), nil
	}
}

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
		return "No cheating fuckface,", nil
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
