package tickets

import (
	"context"
	"emperror.dev/errors"
	"fmt"
	"github.com/cirelion/flint/applications"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/bot/botrest"
	"github.com/cirelion/flint/bot/eventsystem"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	"github.com/cirelion/flint/tickets/models"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"time"
)

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLast(p, p.handleChannelRemoved, eventsystem.EventChannelDelete)
	eventsystem.AddHandlerAsyncLastLegacy(p, p.handleInteractionCreate, eventsystem.EventInteractionCreate)
}

func (p *Plugin) handleInteractionCreate(evt *eventsystem.EventData) {
	ic := evt.InteractionCreate()
	if ic.GuildID == 0 {
		//DM interactions are handled via pubsub
		return
	}

	if ic.Type == discordgo.InteractionMessageComponent {
		customID := ic.MessageComponentData().CustomID
		if customID == applications.TicketSubmit {
			startTicketModal(ic, evt.Session)
		}
		return
	}
	if ic.Type != discordgo.InteractionModalSubmit {
		// Not a modal interaction
		return
	}

	if ic.DataCommand == nil && ic.DataModal == nil {
		// Modal interaction had no data
		return
	}

	if ic.Type == discordgo.InteractionModalSubmit && ic.DataModal.CustomID == TicketModal {
		subject := ic.DataModal.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput)
		question := ic.DataModal.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput)

		config, err := models.FindTicketConfig(evt.Context(), boil.GetContextDB(), ic.GuildID)
		if err != nil {
			logger.Error(err)
			return
		}

		ms, err := bot.GetMember(ic.GuildID, ic.Member.User.ID)
		if err != nil {
			logger.Error(err)
			return
		}

		guild, err := botrest.GetGuild(ic.GuildID)
		if err != nil {
			logger.Error(err)
			return
		}

		_, ticket, err := CreateTicket(evt.Context(), guild, ms, config, subject.Value, question.Value, true)
		if err != nil {
			logger.Error(err)
			return
		}

		err = common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: fmt.Sprintf("Support ticket opened successfully! Click [here](https://discord.com/channels/%d/%d) to go to your ticket.", ticket.GuildID, ticket.ChannelID), Flags: 64},
		})
		if err != nil {
			logger.WithError(err).Error("Failed sending ticket confirm message")
			return
		}
	}
}
func (p *Plugin) handleChannelRemoved(evt *eventsystem.EventData) (retry bool, err error) {
	del := evt.ChannelDelete()

	_, err = models.Tickets(
		models.TicketWhere.ChannelID.EQ(del.Channel.ID),
	).DeleteAll(evt.Context(), common.PQ)

	if err != nil {
		return true, errors.WithStackIf(err)
	}

	return false, nil
}

type TicketUserError string

func (t TicketUserError) Error() string {
	return string(t)
}

const (
	ErrNoTicketCateogry TicketUserError = "No category for ticket channels set"
	ErrMaxOpenTickets   TicketUserError = "You're currently in over 3 open tickets on this server, please close some of the ones you're in."
)

func CreateTicket(ctx context.Context, gs *dstate.GuildSet, ms *dstate.MemberState, conf *models.TicketConfig, topic string, question string, checkMaxTickets bool) (*dstate.GuildSet, *models.Ticket, error) {
	if gs.GetChannel(conf.TicketsChannelCategory) == nil {
		return gs, nil, ErrNoTicketCateogry
	}

	if hasPerms, _ := bot.BotHasPermissionGS(gs, 0, InTicketPerms); !hasPerms {
		return gs, nil, TicketUserError(fmt.Sprintf("The bot is missing one of the following permissions: %s", common.HumanizePermissions(InTicketPerms)))
	}

	if checkMaxTickets {
		inCurrentTickets, err := models.Tickets(
			qm.Where("closed_at IS NULL"),
			qm.Where("guild_id = ?", gs.ID),
			qm.Where("author_id = ?", ms.User.ID)).AllG(ctx)
		if err != nil {
			return gs, nil, err
		}

		count := 0
		for _, v := range inCurrentTickets {
			if gs.GetChannel(v.ChannelID) != nil {
				count++
			}
		}

		if count >= 3 {
			return gs, nil, ErrMaxOpenTickets
		}
	}

	// we manually insert the channel into gs for reliability
	gsCop := *gs
	gsCop.Channels = make([]dstate.ChannelState, len(gs.Channels), len(gs.Channels)+1)
	copy(gsCop.Channels, gs.Channels)

	id, channel, err := createTicketChannel(conf, gs, ms.User.ID)
	if err != nil {
		return gs, nil, err
	}

	// create the db model for it
	dbModel := &models.Ticket{
		GuildID:               gs.ID,
		LocalID:               id,
		ChannelID:             channel.ID,
		Title:                 topic,
		Question:              question,
		CreatedAt:             time.Now(),
		AuthorID:              ms.User.ID,
		AuthorUsernameDiscrim: ms.User.String(),
	}

	err = dbModel.InsertG(ctx, boil.Infer())
	if err != nil {
		return gs, nil, err
	}

	// send the first ticket message
	cs := dstate.ChannelStateFromDgo(channel)

	// insert the channel into gs
	gs = &gsCop
	gs.Channels = append(gs.Channels, cs)

	_, err = common.BotSession.ChannelMessageSendComplex(channel.ID, &discordgo.MessageSend{
		Content: fmt.Sprintf("Hello %s! A moderator will respond to your inquiry shortly!", ms.User.Mention()),
		Embeds: []*discordgo.MessageEmbed{{
			Title:       topic,
			Description: question,
		}},
	})

	if err != nil {
		logger.WithError(err).WithField("guild", gs.ID).Error("failed sending ticket open message")
	}

	return gs, dbModel, nil
}
