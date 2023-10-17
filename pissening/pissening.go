package pissening

import (
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/fun"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	log "github.com/sirupsen/logrus"
	"strconv"
	"time"
)

var (
	Pisstory = &commands.YAGCommand{
		CmdCategory:    commands.CategoryTool,
		Name:           "PisStory",
		Aliases:        []string{"pisshistory", "pisses"},
		Description:    "Shows the users' piss history",
		RequiredArgs:   0,
		DefaultEnabled: false,
		Arguments: []*dcmd.ArgDef{
			{Name: "User", Help: "The user you want to see the screws of", Type: &commands.MemberArg{}},
		},
		RunFunc: getPissHistory,
	}
	StartPissening = &commands.YAGCommand{
		CmdCategory:    commands.CategoryTool,
		Name:           "StartPissening",
		Aliases:        []string{"initpissening"},
		Description:    "Starts the pissening",
		DefaultEnabled: false,
		RunFunc:        startPissening,
	}
	StopPissening = &commands.YAGCommand{
		CmdCategory:    commands.CategoryTool,
		Name:           "StopPissening",
		Aliases:        []string{"endpissening"},
		Description:    "Stops the pissening",
		DefaultEnabled: false,
		RunFunc:        stopPissening,
	}
	EnterPissening = &commands.YAGCommand{
		CmdCategory:         commands.CategoryTool,
		Name:                "EnterPissening",
		Aliases:             []string{"enter", "joinpissening", "join"},
		Description:         "Enters the pissening",
		DefaultEnabled:      false,
		IsResponseEphemeral: true,
		RunFunc:             enterPissening,
	}
	Piss = &commands.YAGCommand{
		CmdCategory:    commands.CategoryTool,
		Name:           "Piss",
		Aliases:        []string{"pee", "pp"},
		Description:    "Pisses in the jar",
		DefaultEnabled: false,
		RunFunc:        piss,
	}
)

func piss(data *dcmd.Data) (interface{}, error) {
	config, err := fun.GetConfig(data.Context(), data.GuildData.GS.ID)
	if err != nil {
		return nil, err
	}

	pisser := getPisser(data.GuildData.GS.ID, data.Author.ID)
	pissening := &Pissening{
		GuildID: data.GuildData.GS.ID,
	}

	err = common.GORM.Model(pissening).Where("pissening_state = ?", Started).First(pissening).Error
	if err == nil {
		handlePiss(config, pissening, pisser, data.Author)
	} else {
		msg, _ := common.BotSession.ChannelMessageSend(config.PChannel, fmt.Sprintf("%s pissed wildly around him, try not to slip!", data.Author.Mention()))
		go func() {
			time.Sleep(30 * time.Second)
			common.BotSession.ChannelMessageDelete(config.PChannel, msg.ID)
		}()
	}

	if data.TriggerType != 3 {
		err = common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
	}

	return nil, nil
}

func enterPissening(data *dcmd.Data) (interface{}, error) {
	config, err := fun.GetConfig(data.Context(), data.GuildData.GS.ID)
	if err != nil {
		return nil, err
	}

	pissening := &Pissening{
		GuildID: data.GuildData.GS.ID,
	}

	err = common.GORM.Model(pissening).Where("pissening_state = ?", Initialized).First(pissening).Error
	if err == nil {
		if common.ContainsInt64Slice(pissening.Pissers, data.Author.ID) {
			if data.TriggerType != 3 {
				err = common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
			}

			return nil, nil
		}

		pissening.Pissers = append(pissening.Pissers, data.Author.ID)
		pissening.MessageID = handleMessage(config.PChannel, pissening, false)
		common.GORM.Model(pissening).Update(pissening)

		pisser := getPisser(data.GuildData.GS.ID, data.Author.ID)
		pisser.CompetedPissenings++
		pisser.ActivePissening = pissening
		common.GORM.Model(pisser).Update(pisser)
	}

	if data.TriggerType != 3 {
		err = common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
	}

	return nil, err
}

func startPissening(data *dcmd.Data) (interface{}, error) {
	config, err := fun.GetConfig(data.Context(), data.GuildData.GS.ID)
	if err != nil {
		return nil, err
	}

	pissening := &Pissening{
		GuildID: data.GuildData.GS.ID,
	}
	previousPisser := &Pisser{
		GuildID: data.GuildData.GS.ID,
	}
	err = common.GORM.Model(previousPisser).Where("has_p_role = true").Find(previousPisser).Error
	if err == nil {
		common.BotSession.GuildMemberRoleRemove(previousPisser.GuildID, previousPisser.UserID, config.PRole)
	}

	err = common.GORM.Model(pissening).Where("pissening_state != ?", Ended).First(pissening).Error
	if err == nil {
		if pissening.AuthorID != data.Author.ID {
			return nil, nil
		}

		pissening.PisseningState = Started
		pissening.MessageID = handleMessage(config.PChannel, pissening, true)

		common.GORM.Model(pissening).Update(pissening)
	} else {
		pisser := getPisser(data.GuildData.GS.ID, data.Author.ID)
		pissening.Pissers = append(pissening.Pissers, pisser.UserID)
		pissening.AuthorID = data.Author.ID

		msg, msgErr := common.BotSession.ChannelMessageSendComplex(config.PChannel, &discordgo.MessageSend{
			Embeds:     generatePisseningEmbed(pissening, false),
			Components: generatePisseningButtons(pissening),
		})

		if msgErr != nil {
			return nil, msgErr
		}

		pissening.MessageID = msg.ID

		common.GORM.Model(pissening).Save(pissening)
		pisser.ActivePissening = pissening
		pisser.CompetedPissenings++

		common.GORM.Model(pisser).Update(pisser)
	}

	if data.TriggerType != 3 {
		err = common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
	}

	return nil, err
}

func stopPissening(data *dcmd.Data) (interface{}, error) {
	config, err := fun.GetConfig(data.Context(), data.GuildData.GS.ID)
	if err != nil {
		return nil, err
	}

	pissening := &Pissening{
		GuildID: data.GuildData.GS.ID,
	}

	err = common.GORM.Model(pissening).Where("author_id = ?", data.Author.ID).Where("pissening_state != ?", Ended).First(pissening).Error
	if err == nil {
		pissening.PisseningState = Ended
		pissening.MessageID = handleMessage(config.PChannel, pissening, true)

		common.GORM.Model(pissening).Update(pissening)
	}

	if data.TriggerType != 3 {
		err = common.BotSession.ChannelMessageDelete(data.ChannelID, data.TraditionalTriggerData.Message.ID)
	}

	return nil, err
}

func handleMessage(channelID int64, pissening *Pissening, updateTimestamp bool) int64 {
	msg, err := common.BotSession.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID:         pissening.MessageID,
		Channel:    channelID,
		Embeds:     generatePisseningEmbed(pissening, updateTimestamp),
		Components: generatePisseningButtons(pissening),
	})

	if err != nil {
		msg, err = common.BotSession.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
			Embeds:     generatePisseningEmbed(pissening, updateTimestamp),
			Components: generatePisseningButtons(pissening),
		})

		if err != nil {
			log.Error(err)
		}
	}

	return msg.ID
}

func generatePisseningEmbed(pissening *Pissening, updateTimestamp bool) []*discordgo.MessageEmbed {
	limit := len(pissening.Pissers) * 5
	embed := &discordgo.MessageEmbed{
		Title: "The Pissening",
		Color: 15974714,
	}

	switch pissening.PisseningState {
	case Initialized:
		embed.Description = "Join the Pissening before it's too late"
		embed.Footer = &discordgo.MessageEmbedFooter{Text: "Pissening hasn't started yet"}
	case Started:
		embed.Description = "The Pissening has begun"
		embed.Footer = &discordgo.MessageEmbedFooter{Text: "Pissening started"}
	case Ended:
		member, err := bot.GetMember(pissening.GuildID, pissening.JarSpiller)

		if err != nil {
			embed.Description = "The pissening has ended prematurely, the jar remains half-full"
		} else {
			embed.Description = fmt.Sprintf("The pissening has ended! Shame on %s!", member.User.Mention())
		}

		embed.Footer = &discordgo.MessageEmbedFooter{Text: "Pissening ended"}
	}

	if updateTimestamp {
		embed.Timestamp = time.Now().Format(time.RFC3339)
	}

	embed.Description += fmt.Sprintf("\n\n**Participants**: %d", len(pissening.Pissers))

	for _, pisser := range pissening.Pissers {
		member, err := bot.GetMember(pissening.GuildID, pisser)
		if err != nil {
			log.Error(err)
			continue
		}

		embed.Description += fmt.Sprintf("\n- %s", member.User.Mention())
		if pissening.PisseningState == Ended {
			count := 0
			for _, peer := range pissening.Pisses {
				if peer == pisser {
					count++
				}
			}

			embed.Description += fmt.Sprintf(" - Pissed %d times", count)
		}

	}

	if limit-len(pissening.Pisses) <= 5 && pissening.PisseningState == Started {
		embed.Description += "\n\nHold it! The jar is almost full!"
	}

	return []*discordgo.MessageEmbed{embed}
}

func generatePisseningButtons(pissening *Pissening) []discordgo.MessageComponent {
	var components []discordgo.MessageComponent

	if pissening.PisseningState == Initialized {
		components = append(components, discordgo.Button{
			Label:    "Join the pissening",
			Style:    discordgo.SecondaryButton,
			Disabled: pissening.PisseningState != Initialized,
			CustomID: JoinID,
		})
		components = append(components, discordgo.Button{
			Label:    "Start",
			Style:    discordgo.SuccessButton,
			Disabled: pissening.PisseningState != Initialized,
			CustomID: StartID,
		})
	}

	if pissening.PisseningState == Started {
		components = append(components, discordgo.Button{
			Style:    discordgo.PrimaryButton,
			Disabled: pissening.PisseningState != Started,
			Emoji: discordgo.ComponentEmoji{
				ID:   957804887100497970,
				Name: "wearydrops",
			},
			CustomID: PissID,
		})
		components = append(components, discordgo.Button{
			Label:    "Stop",
			Style:    discordgo.DangerButton,
			Disabled: pissening.PisseningState != Started,
			CustomID: StopID,
		})
	}

	if len(components) > 0 {
		return []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: components,
			},
		}
	}

	return []discordgo.MessageComponent{}
}

func getPisser(guildID int64, userID int64) Pisser {
	pisser := Pisser{
		UserID:  userID,
		GuildID: guildID,
	}
	err := common.GORM.Model(&pisser).FirstOrCreate(&pisser).Error

	if err != nil {
		log.Error(err)
	}

	return pisser
}
func getPissHistory(data *dcmd.Data) (interface{}, error) {
	guildID := data.GuildData.GS.ID
	user := data.Args[0].User()
	authorMember, memberErr := bot.GetMember(guildID, data.Author.ID)
	var member *dstate.MemberState

	if user == nil {
		user = data.Author
		member = authorMember
	} else {
		member, memberErr = bot.GetMember(guildID, user.ID)
	}

	if memberErr != nil {
		return nil, memberErr
	}

	pisser := getPisser(guildID, user.ID)

	_, err := common.BotSession.ChannelMessageSendEmbed(data.ChannelID, &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("Pissening Stats - %s", bot.GetName(member)),
		Timestamp: time.Now().Format(time.RFC3339),
		Color:     15974714,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: member.User.AvatarURL("256"),
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text:    fmt.Sprintf("Asked by %s", bot.GetName(authorMember)),
			IconURL: discordgo.EndpointUserAvatar(data.Author.ID, data.Author.Avatar),
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "Total pisses",
				Value: strconv.FormatInt(pisser.TotalPisses, 10),
			},
			{
				Name:  "Jars spilled",
				Value: strconv.FormatInt(pisser.JarsSpilled, 10),
			},
			{
				Name:  "Pissenings participated in",
				Value: strconv.FormatInt(pisser.CompetedPissenings, 10),
			},
		}})

	return nil, err
}
