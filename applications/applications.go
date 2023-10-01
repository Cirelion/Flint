package applications

import (
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
)

var (
	FavouriteConversationsID       = "Favourite conversations"
	EditedFavouriteConversationsID = "Immersion/Optimization"
	ConversationLink               = "Conversation link"
	ConversationReason             = "Conversation reason"
	Select                         = "application_select"
	MiniModSubmit                  = "Mini mod submission"
	TicketSubmit                   = "Ticket submission"
	MovieSuggestion                = "Movie suggestion"
	MoveHostSubmit                 = "Host/stream application"
	Apply                          = &commands.YAGCommand{
		CmdCategory:               commands.CategoryTool,
		Name:                      "PostApplicationEmbed",
		Description:               "Sends a DM for applications",
		DefaultEnabled:            true,
		ApplicationCommandEnabled: true,
		RequireDiscordPerms:       []int64{discordgo.PermissionKickMembers},
		RequiredDiscordPermsHelp:  "KickMembers",
		RequiredArgs:              1,
		RequireBotPerms:           [][]int64{{discordgo.PermissionManageChannels}},
		Arguments: []*dcmd.ArgDef{
			{Name: "Variant", Help: "Type of application embed you want to post [conv|mod|event|ticket]", Type: dcmd.String},
		},
		IsResponseEphemeral: true,
		RunFunc:             startApplication,
	}
)

func startApplication(data *dcmd.Data) (interface{}, error) {
	message := generateApplicationMessage(data.Args[0].Str())
	if message == nil {
		return "Incorrect variant set, possible variants are: [conv|mod|event]", nil
	}

	_, err := common.BotSession.ChannelMessageSendComplex(data.ChannelID, message)
	if err != nil {
		return "Failed setting application embed", err
	}

	return "Application embed posted", nil
}

func generateApplicationMessage(variant string) *discordgo.MessageSend {
	var title string
	var color int
	var description string
	var components []discordgo.MessageComponent

	switch variant {
	case "conv":
		title = "Favourite conversation"
		color = 0x57728e
		description = "Submit your conversation link here!\n\n" +
			"This will get sent directly to Pat to use in training our new proprietary model! SFW and NSFW are allowed, your conversations won't be shared with anyone else. \n\n" +
			"Help us out make Unhinged become the best it can be!"
		components = []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    Select,
				Placeholder: "Make a selection",
				Options: []discordgo.SelectMenuOption{
					{
						Label:       "Favourite conversation",
						Value:       "favourite_conversations",
						Description: "Favourite unedited conversations!",
						Default:     false,
					},
					{
						Label:       "Immersion/Optimization",
						Value:       "edited_conversations",
						Description: "Conversations improved through the message editing feature",
						Default:     false,
					},
				},
			},
		}
	case "event":
		title = "Event forms"
		description = "Suggest movies for movie nights or apply to host/stream for us!"
		color = 0xd64848
		components = []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Suggest a movie",
				Style:    discordgo.DangerButton,
				CustomID: MovieSuggestion,
			},
			discordgo.Button{
				Label:    "Sign up to host",
				Style:    discordgo.SecondaryButton,
				CustomID: MoveHostSubmit,
			},
		}
	case "mod":
		title = "Apply to become a mini-mod here!"
		description = "Simply press the button and the process will start!"
		color = 0x57728e
		components = []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Apply here",
				Style:    discordgo.PrimaryButton,
				CustomID: MiniModSubmit,
			},
		}
	case "ticket":
		title = "Contact Staff"
		description = "Do you need talk to mods in confidence, double-check if your bot is safe for publication or need to get into contact with the mods for any other reason?\n\nPlease click the button below to open a support ticket!"
		color = 0x62c65f
		components = []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Open support ticket",
				Style:    discordgo.SuccessButton,
				CustomID: TicketSubmit,
			},
		}
	default:
		return nil
	}

	return &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{
			{Title: title, Description: description, Color: color},
		},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: components,
			},
		},
	}
}
