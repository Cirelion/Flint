package applications

import (
	"fmt"
	"github.com/botlabs-gg/yagpdb/v2/bot"
	"github.com/botlabs-gg/yagpdb/v2/bot/eventsystem"
	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/lib/discordgo"
	"github.com/botlabs-gg/yagpdb/v2/moderation"
	"time"
)

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Applications",
		SysName:  "applications",
		Category: common.PluginCategoryMisc,
	}
}

var logger = common.GetPluginLogger(&Plugin{})

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, bot.ConcurrentEventHandler(handleApplicationInteractionCreate), eventsystem.EventInteractionCreate)
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		Apply,
	)
}

func handleApplicationStart(evt *eventsystem.EventData) {
	ic := evt.EvtInterface.(*discordgo.InteractionCreate)
	data := ic.MessageComponentData()

	if len(data.Values) > 0 {
		switch data.Values[0] {
		case "favourite_conversations":
			startFavouriteConversationModal(ic, evt.Session, FavouriteConversationsID, "Favourite conversations")
		case "edited_conversations":
			startFavouriteConversationModal(ic, evt.Session, EditedFavouriteConversationsID, "Immersion/Optimization")
		}
	} else {
		switch data.CustomID {
		case MiniModSubmit:
			startMiniModModal(ic, evt.Session)
		case MovieSuggestion:
			startMovieSuggestionModal(ic, evt.Session)
		case MoveHostSubmit:
			startMovieHostModal(ic, evt.Session)
		}
	}
}

func handleApplicationInteractionCreate(evt *eventsystem.EventData) {
	ic := evt.EvtInterface.(*discordgo.InteractionCreate)
	config, _ := moderation.GetConfig(ic.GuildID)

	if ic.Type == discordgo.InteractionMessageComponent {
		handleApplicationStart(evt)
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

	if ic.Type == discordgo.InteractionModalSubmit {
		embed := &discordgo.MessageEmbed{
			Title:       ic.DataModal.CustomID,
			Description: fmt.Sprintf("Application from %s received", ic.Member.User.Mention()),
			Color:       16777215,
			Author: &discordgo.MessageEmbedAuthor{
				Name:    ic.Member.User.Username,
				IconURL: discordgo.EndpointUserAvatar(ic.Member.User.ID, ic.Member.User.Avatar),
			},
			Footer: &discordgo.MessageEmbedFooter{
				Text: "Application submitted",
			},
			Timestamp: time.Now().Format(time.RFC3339),
		}

		for _, modalComponent := range ic.DataModal.Components {
			input := modalComponent.(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput)
			embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
				Name:  input.CustomID,
				Value: input.Value,
			})
		}

		_, err := common.BotSession.ChannelMessageSendEmbed(getLogChannel(config, ic.DataModal.CustomID), embed)
		if err != nil {
			logger.WithError(err).Error("Failed sending application.")
			return
		}

		err = common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Flags: 64},
		})
		if err != nil {
			logger.WithError(err).Error("Failed creating Application Deferred Response")
			return
		}

		_, err = common.BotSession.FollowupMessageCreate(&ic.Interaction, true, &discordgo.WebhookParams{
			Content:         "Your application has been submitted successfully!",
			AllowedMentions: &discordgo.AllowedMentions{},
			Flags:           64,
		})
		if err != nil {
			logger.WithError(err).Error("Failed creating Application FollowupMessage")
			return
		}
	}
}

func getLogChannel(config *moderation.Config, customID string) int64 {
	switch true {
	case customID == EditedFavouriteConversationsID || customID == FavouriteConversationsID:
		return config.IntConversationSubmissionChannel()
	case customID == MiniModSubmit:
		return config.IntModApplicationSubmissionChannel()
	case customID == MovieSuggestion || customID == MoveHostSubmit:
		return config.IntEventSubmissionChannel()
	}

	return 0
}
