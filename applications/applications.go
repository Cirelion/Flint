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
			{Name: "Variant", Help: "Type of application embed you want to post [conv|mod|event]", Type: dcmd.String},
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
	var description string
	var components []discordgo.MessageComponent

	switch variant {
	case "conv":
		title = "Favourite conversation"
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
		components = []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Suggest a movie",
				Style:    discordgo.PrimaryButton,
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
		components = []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Apply here",
				Style:    discordgo.PrimaryButton,
				CustomID: MiniModSubmit,
			},
		}
	default:
		return nil
	}

	return &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{
			{Title: title, Description: description, Color: 0x57728e},
		},
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: components,
			},
		},
	}
}
