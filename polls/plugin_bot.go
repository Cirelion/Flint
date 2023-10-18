package polls

import (
	"errors"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/bot/eventsystem"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/discordgo"
	"reflect"
	"strings"
	"time"
)

var (
	logger          = common.GetPluginLogger(&Plugin{})
	pollYay         = "poll_yay"
	pollNay         = "poll_nay"
	strawPollSelect = "straw_poll_select_menu"
)

var ErrNoResults = errors.New("no results")

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Poll messages",
		SysName:  "poll_messages",
		Category: common.PluginCategoryMisc,
	}
}

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})
	common.GORM.AutoMigrate(&ContestRound{}, &PollMessage{}, &SelectMenuOption{}, &Vote{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, handleInteractionCreate, eventsystem.EventInteractionCreate)

	rows, err := common.GORM.Model(&ContestRound{}).Where("active = ?", common.BoolToPointer(true)).Rows()
	if err != nil {
		logger.Error(err)
		return
	}

	for rows.Next() {
		var contestRound ContestRound
		err = common.GORM.ScanRows(rows, &contestRound)
		if err != nil {
			logger.Error(err)
			return
		}

		go contestRound.handleContestTimer()
	}
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		Poll,
		StrawPoll,
		EndPoll,
		StartContestRound,
	)
}

func handleInteractionCreate(evt *eventsystem.EventData) {
	ic := evt.InteractionCreate()
	if ic.Type != discordgo.InteractionMessageComponent {
		return
	}
	if ic.GuildID == 0 {
		//DM interactions are handled via pubsub
		return
	}

	customID := ic.MessageComponentData().CustomID

	if customID == pollNay || customID == pollYay || customID == strawPollSelect {
		var votes []string
		var err error

		poll := &PollMessage{
			MessageID: ic.Message.ID,
		}
		err = common.GORM.Model(&poll).First(&poll).Error

		if err != nil {
			logger.Error(err)
			return
		}

		switch customID {
		case pollYay:
			votes = append(votes, "0")
			handleVote(ic, Vote{PollMessageID: poll.MessageID, UserID: ic.Member.User.ID, Vote: strings.Join(votes, ", ")})
		case pollNay:
			votes = append(votes, "1")
			handleVote(ic, Vote{PollMessageID: poll.MessageID, UserID: ic.Member.User.ID, Vote: strings.Join(votes, ", ")})
		case strawPollSelect:
			values := ic.Data.(discordgo.MessageComponentInteractionData).Values

			for _, value := range values {
				votes = append(votes, value)
			}

			handleVote(ic, Vote{PollMessageID: poll.MessageID, UserID: ic.Member.User.ID, Vote: strings.Join(votes, ", ")})
		}
	} else if strings.Contains(customID, "contest_round") {
		contestRound := &ContestRound{
			MessageID: ic.Message.ID,
		}

		err := common.GORM.Model(contestRound).First(contestRound).Error
		if err != nil {
			logger.Error(err)
			return
		}

		handleContestRoundVote(ic, Vote{PollMessageID: contestRound.MessageID, UserID: ic.Member.User.ID, Vote: customID})
	}
}

func handleContestRoundVote(ic *discordgo.InteractionCreate, vote Vote) {
	if ic.Member != nil && ic.Member.User.ID == common.BotUser.ID {
		return
	}

	if ic.User != nil && ic.User.ID == common.BotUser.ID {
		return
	}

	contestRound := &ContestRound{
		MessageID: ic.Message.ID,
	}
	err := common.GORM.Model(&contestRound).Preload("Votes").First(&contestRound).Error
	if err != nil {
		logger.Error(err)
	} else {
		contestRound.HandleVoteButtonClick(ic, vote)
	}
}

func handleVote(ic *discordgo.InteractionCreate, vote Vote) {
	if ic.Member != nil && ic.Member.User.ID == common.BotUser.ID {
		return
	}

	if ic.User != nil && ic.User.ID == common.BotUser.ID {
		return
	}

	poll := &PollMessage{
		MessageID: ic.Message.ID,
	}
	err := common.GORM.Model(&poll).Preload("Options").Preload("Votes").First(&poll).Error
	if err != nil {
		logger.Error(err)
	} else {
		poll.HandleVoteButtonClick(ic, vote)
	}
}

func (c ContestRound) HandleVoteButtonClick(ic *discordgo.InteractionCreate, vote Vote) {
	err := common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		logger.Error(err)
		return
	}

	var votes []Vote
	var repeat = false

	if len(c.Votes) > 0 {
		for _, value := range c.Votes {
			if value.UserID != vote.UserID {
				votes = append(votes, value)
			} else if reflect.DeepEqual(value.Vote, vote.Vote) {
				repeat = true
				err = common.GORM.Model(&c).Association("Votes").Delete(&vote).Error
			}
		}
	}

	if !repeat {
		votes = append(votes, vote)
	}

	if err != nil && err != ErrNoResults {
		logger.WithError(err).WithField("guild", c.GuildID).Error("failed setting vote")
		return
	}

	c.Votes = votes

	ic.Message.Embeds[2] = generateContestRoundPollEmbed(ic.Message.Embeds[2].Title, c.FirstPost, c.SecondPost, votes, c.CreatedAt.Add(time.Hour*24), false)
	_, _ = common.BotSession.ChannelMessageEditEmbedList(ic.ChannelID, ic.Message.ID, ic.Message.Embeds)

	err = common.GORM.Model(&c).Update(&c).Error
	if err != nil {
		switch code, _ := common.DiscordError(err); code {
		case discordgo.ErrCodeUnknownChannel, discordgo.ErrCodeUnknownMessage, discordgo.ErrCodeMissingAccess, discordgo.ErrCodeMissingPermissions:
			c.Broken = true
		default:
			logger.WithError(err).WithField("guild", c.GuildID).Error("failed updating contest round message")
		}
	}
}

func (p *PollMessage) HandleVoteButtonClick(ic *discordgo.InteractionCreate, vote Vote) {
	err := common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
	if err != nil {
		return
	}

	var votes []Vote
	var repeat = false

	if len(p.Votes) > 0 {
		for _, value := range p.Votes {
			if value.UserID != vote.UserID {
				votes = append(votes, value)
			} else if reflect.DeepEqual(value.Vote, vote.Vote) {
				repeat = true
				err = common.GORM.Model(&p).Association("Votes").Delete(&vote).Error
			}
		}
	}

	if !repeat {
		votes = append(votes, vote)
	}

	var newMsg *discordgo.MessageEmbed
	author, err := bot.GetMember(p.GuildID, p.AuthorID)

	if p.IsStrawPoll {
		newMsg, err = StrawPollEmbed(p.Question, p.Options, &author.User, votes)
	} else {
		newMsg, err = PollEmbed(p.Question, &author.User, votes)
	}

	if err != nil && err != ErrNoResults {
		logger.WithError(err).WithField("guild", p.GuildID).Error("failed setting vote")
		return
	}

	if newMsg == nil {
		// No change...
		return
	}

	p.Votes = votes

	newMsg.Timestamp = time.Now().Format(time.RFC3339)

	_, err = common.BotSession.ChannelMessageEditEmbed(ic.ChannelID, ic.Message.ID, newMsg)
	err = common.GORM.Model(&p).Update(&p).Error
	if err != nil {
		switch code, _ := common.DiscordError(err); code {
		case discordgo.ErrCodeUnknownChannel, discordgo.ErrCodeUnknownMessage, discordgo.ErrCodeMissingAccess, discordgo.ErrCodeMissingPermissions:
			p.Broken = true
		default:
			logger.WithError(err).WithField("guild", p.GuildID).Error("failed updating poll message")
		}
	}
}

func (c ContestRound) handleContestTimer() {
	ticker := time.NewTicker(time.Minute * 1)

	for {
		<-ticker.C
		if time.Since(c.CreatedAt) >= time.Hour*24 {
			err := common.GORM.Model(&c).Preload("Votes").First(&c).Error
			if err != nil {
				logger.Error(err)
				return
			}

			message, err := common.BotSession.ChannelMessage(c.ChannelID, c.MessageID)
			if err != nil {
				logger.Error(err)
				return
			}

			message.Embeds[2] = generateContestRoundPollEmbed(message.Embeds[2].Title, c.FirstPost, c.SecondPost, c.Votes, c.CreatedAt.Add(time.Hour*24), true)

			msgEdit := &discordgo.MessageEdit{
				ID:         c.MessageID,
				Channel:    c.ChannelID,
				Embeds:     message.Embeds,
				Components: []discordgo.MessageComponent{},
			}

			_, err = common.BotSession.ChannelMessageEditComplex(msgEdit)
			if err != nil {
				logger.WithError(err).WithField("guild", c.GuildID).Error("failed closing round")
			}

			c.Active = common.BoolToPointer(false)
			err = common.GORM.Model(&c).Update(c).Error
			if err != nil {
				logger.Error(err)
				return
			}

			ticker.Stop()
		}
	}
}
