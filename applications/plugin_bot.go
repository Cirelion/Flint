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

var (
	moderationQuestion1 = "Timezone and available hours in UTC"
	moderationQuestion2 = "Experience with Discord moderation and tools"
	moderationQuestion3 = "Experience with informing & handling disputes"
	moderationQuestion4 = "How do you treat big vs minor rule violations"
	moderationQuestion5 = "Why should you be selected as mini moderator?"
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
			Content:         "Your application has been submitted successfully! Thanks for helping our models become the very best!",
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
	if customID == EditedFavouriteConversationsID || customID == FavouriteConversationsID {
		return config.IntConversationSubmissionChannel()
	}

	if customID == "Mini mod submission" {
		return config.IntModApplicationSubmissionChannel()
	}

	return 0
}

func startMiniModModal(ic *discordgo.InteractionCreate, session *discordgo.Session) {
	params := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "Mini mod submission",
			Title:    "Mini-mod application",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  moderationQuestion1,
							Label:     "Timezone and available hours in UTC",
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 1000,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  moderationQuestion2,
							Label:     "Experience with Discord moderation and tools",
							Style:     discordgo.TextInputParagraph,
							Required:  true,
							MaxLength: 1000,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  moderationQuestion3,
							Label:     "Experience with informing & handling disputes",
							Style:     discordgo.TextInputParagraph,
							Required:  true,
							MaxLength: 1000,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  moderationQuestion4,
							Label:     "How do you treat big vs minor rule violations",
							Style:     discordgo.TextInputParagraph,
							Required:  true,
							MaxLength: 1000,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  moderationQuestion5,
							Label:     "Why should you be selected as mini moderator?",
							Style:     discordgo.TextInputParagraph,
							Required:  true,
							MaxLength: 1000,
						},
					},
				},
			},
			Flags: 64,
		},
	}

	err := session.CreateInteractionResponse(ic.ID, ic.Token, params)
	if err != nil {
		logger.Error(err)
		return
	}
}
func startFavouriteConversationModal(ic *discordgo.InteractionCreate, session *discordgo.Session, customID string, title string) {
	params := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: customID,
			Title:    title,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    ConversationLink,
							Label:       "The link to the conversation",
							Style:       discordgo.TextInputShort,
							Required:    true,
							MinLength:   40,
							MaxLength:   100,
							Placeholder: "https://wwww.unhinged.ai/chat?conversationId=65185e2870555d99bd092765",
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    ConversationReason,
							Label:       "What makes this conversation good?",
							Style:       discordgo.TextInputParagraph,
							Required:    false,
							MaxLength:   500,
							Placeholder: "The bot stays in character very well and writes coherent and original answers.",
						},
					},
				},
			},
			Flags: 64,
		},
	}

	err := session.CreateInteractionResponse(ic.ID, ic.Token, params)
	if err != nil {
		logger.Error(err)
		return
	}
}
