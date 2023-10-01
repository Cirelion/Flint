package applications

import "github.com/cirelion/flint/lib/discordgo"

var (
	moderationQuestion1 = "Timezone and available hours in UTC"
	moderationQuestion2 = "Experience with Discord moderation and tools"
	moderationQuestion3 = "Experience with informing & handling disputes"
	moderationQuestion4 = "How do you treat big vs minor rule violations"
	moderationQuestion5 = "Why should you be selected as mini moderator?"
)

var (
	movieSuggestionQuestion1 = "Movie title"
	movieSuggestionQuestion2 = "Movie genre(s)"
	movieSuggestionQuestion3 = "Where is it streamable?"
	movieSuggestionQuestion4 = "Description of movie/why you want to watch it"
)

var (
	movieHostQuestion1 = "Do you have good, stable, reliable internet?"
	movieHostQuestion2 = "Have you successfully streamed movies before?"
	movieHostQuestion3 = "What streaming services do you have?"
	movieHostQuestion4 = "What days and time (UTC) are you available?"
	movieHostQuestion5 = "When wouldn't you be able to stream?"
)

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
							Label:     moderationQuestion1,
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
							Label:     moderationQuestion2,
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
							Label:     moderationQuestion3,
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
							Label:     moderationQuestion4,
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
							Label:     moderationQuestion5,
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

func startMovieSuggestionModal(ic *discordgo.InteractionCreate, session *discordgo.Session) {
	params := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: MovieSuggestion,
			Title:    MovieSuggestion,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieSuggestionQuestion1,
							Label:     movieSuggestionQuestion1,
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 50,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieSuggestionQuestion2,
							Label:     movieSuggestionQuestion2,
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 50,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieSuggestionQuestion3,
							Label:     movieSuggestionQuestion3,
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 50,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieSuggestionQuestion4,
							Label:     movieSuggestionQuestion4,
							Style:     discordgo.TextInputParagraph,
							Required:  false,
							MaxLength: 200,
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
func startMovieHostModal(ic *discordgo.InteractionCreate, session *discordgo.Session) {
	params := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: MoveHostSubmit,
			Title:    MoveHostSubmit,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieHostQuestion1,
							Label:     movieHostQuestion1,
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 50,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieHostQuestion2,
							Label:     movieHostQuestion2,
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 50,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieHostQuestion3,
							Label:     movieHostQuestion3,
							Style:     discordgo.TextInputShort,
							Required:  true,
							MaxLength: 50,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieHostQuestion4,
							Label:     movieHostQuestion4,
							Style:     discordgo.TextInputShort,
							Required:  false,
							MaxLength: 200,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:  movieHostQuestion5,
							Label:     movieHostQuestion5,
							Style:     discordgo.TextInputParagraph,
							Required:  false,
							MaxLength: 200,
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
