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
	common.GORM.AutoMigrate(&PollMessage{}, &SelectMenuOption{}, &Vote{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, handleInteractionCreate, eventsystem.EventInteractionCreate)
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		Poll,
		StrawPoll,
		EndPoll,
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

	var vote []string
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
		vote = append(vote, "0")
		handleVote(ic, Vote{PollMessageID: poll.MessageID, UserID: ic.Member.User.ID, Vote: strings.Join(vote, ", ")})
	case pollNay:
		vote = append(vote, "1")
		handleVote(ic, Vote{PollMessageID: poll.MessageID, UserID: ic.Member.User.ID, Vote: strings.Join(vote, ", ")})
	case strawPollSelect:
		var votes []string
		values := ic.Data.(discordgo.MessageComponentInteractionData).Values

		for _, value := range values {
			votes = append(votes, value)
		}
		vote = append(vote, votes...)
		handleVote(ic, Vote{PollMessageID: poll.MessageID, UserID: ic.Member.User.ID, Vote: strings.Join(vote, ", ")})
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
