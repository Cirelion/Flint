package polls

import (
	"errors"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/botlabs-gg/yagpdb/v2/bot"
	"github.com/botlabs-gg/yagpdb/v2/bot/eventsystem"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/common/pubsub"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
)

var (
	logger                = common.GetPluginLogger(&Plugin{})
	activePollMessagesMap = make(map[int64]*PollMessage)
	menusLock             sync.Mutex
	pollYay               = "poll_yay"
	pollNay               = "poll_nay"
	strawpollSelect       = "strawpoll_selectmenu"
)

var ErrNoResults = errors.New("no results")

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Poll vote messages",
		SysName:  "pollvotemessages",
		Category: common.PluginCategoryMisc,
	}
}

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, handleInteractionCreate, eventsystem.EventInteractionCreate)

	// this just handles interaction events from DMS
	pubsub.AddHandler("dm_interaction", func(evt *pubsub.Event) {
		dataCast := evt.Data.(*discordgo.InteractionCreate)
		if dataCast.Type != discordgo.InteractionMessageComponent {
			return
		}

		var vote []int

		switch dataCast.MessageComponentData().CustomID {
		case pollYay:
			vote = append(vote, 0)
			handleVote(dataCast, Vote{userID: dataCast.User.ID, vote: vote})
		case pollNay:
			vote = append(vote, 1)
			handleVote(dataCast, Vote{userID: dataCast.User.ID, vote: vote})
		case strawpollSelect:
			var votes []int
			values := dataCast.Data.(discordgo.MessageComponentInteractionData).Values

			for _, value := range values {
				v, _ := strconv.ParseInt(value, 10, 0)
				votes = append(votes, int(v))
			}
			vote = append(vote, votes...)
			handleVote(dataCast, Vote{userID: dataCast.Member.User.ID, vote: vote})
		}
	}, discordgo.InteractionCreate{})
}

type PollMessage struct {
	// immutable fields, safe to access without a lock, don't write to these, i dont see why you would need to either...
	MessageID int64
	ChannelID int64
	GuildID   int64

	// mutable fields
	Votes        []Vote
	LastResponse *discordgo.MessageEmbed
	HandleVote   func(p *PollMessage, votes []Vote) (*discordgo.MessageEmbed, error)
	Broken       bool

	stopped        bool
	stopCh         chan bool
	lastUpdateTime time.Time
	mu             sync.Mutex
}
type PollFunc func(p *PollMessage, votes []Vote) (*discordgo.MessageEmbed, error)

type Vote struct {
	userID int64
	vote   []int
}

func (p *PollMessage) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return
	}

	p.stopped = true
	close(p.stopCh)
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

	var vote []int

	switch ic.MessageComponentData().CustomID {
	case pollYay:
		vote = append(vote, 0)
		handleVote(ic, Vote{userID: ic.Member.User.ID, vote: vote})
	case pollNay:
		vote = append(vote, 1)
		handleVote(ic, Vote{userID: ic.Member.User.ID, vote: vote})
	case strawpollSelect:
		var votes []int
		values := ic.Data.(discordgo.MessageComponentInteractionData).Values

		for _, value := range values {
			v, _ := strconv.ParseInt(value, 10, 0)
			votes = append(votes, int(v))
		}
		vote = append(vote, votes...)
		handleVote(ic, Vote{userID: ic.Member.User.ID, vote: vote})
	}
}

func handleVote(ic *discordgo.InteractionCreate, vote Vote) {
	if ic.Member != nil && ic.Member.User.ID == common.BotUser.ID {
		return
	}

	if ic.User != nil && ic.User.ID == common.BotUser.ID {
		return
	}

	menusLock.Lock()
	if paginatedMessage, ok := activePollMessagesMap[ic.Message.ID]; ok {
		menusLock.Unlock()
		paginatedMessage.HandleVoteButtonClick(ic, vote)
		return
	}
	menusLock.Unlock()
}

func (p *PollMessage) HandleVoteButtonClick(ic *discordgo.InteractionCreate, vote Vote) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Pong the interaction
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
			if value.userID != vote.userID {
				votes = append(votes, value)
			} else if reflect.DeepEqual(value.vote, vote.vote) {
				repeat = true
			}
		}
	}

	if !repeat {
		votes = append(votes, vote)
	}

	newMsg, err := p.HandleVote(p, votes)

	if err != nil {
		if err == ErrNoResults {
			newMsg = p.LastResponse
			logger.Println("Vote updated")
		} else {
			logger.WithError(err).WithField("guild", p.GuildID).Error("failed setting vote")
			return
		}
	}

	if newMsg == nil {
		// No change...
		return
	}
	p.LastResponse = newMsg
	p.lastUpdateTime = time.Now()

	p.Votes = votes

	newMsg.Timestamp = time.Now().Format(time.RFC3339)

	_, err = common.BotSession.ChannelMessageEditEmbed(ic.ChannelID, ic.Message.ID, newMsg)

	if err != nil {
		switch code, _ := common.DiscordError(err); code {
		case discordgo.ErrCodeUnknownChannel, discordgo.ErrCodeUnknownMessage, discordgo.ErrCodeMissingAccess, discordgo.ErrCodeMissingPermissions:
			p.Broken = true
		default:
			logger.WithError(err).WithField("guild", p.GuildID).Error("failed updating poll message")
		}
	}
}
