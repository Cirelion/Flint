package roast

import (
	"fmt"
	"html"

	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
)

var Command = &commands.YAGCommand{
	CmdCategory: commands.CategoryFun,
	Name:        "Roast",
	Aliases:     []string{"insult"},
	Description: "Sends a random roast",
	Arguments: []*dcmd.ArgDef{
		{Name: "Target", Type: dcmd.User},
	},
	DefaultEnabled:            true,
	ApplicationCommandEnabled: true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		target := "a random person nearby"
		if data.Args[0].Value != nil {
			target = data.Args[0].Value.(*discordgo.User).Username
		}
		embed := &discordgo.MessageEmbed{}
		embed.Title = fmt.Sprintf(`%s roasted %s`, data.Author.Username, target)
		embed.Description = html.UnescapeString(randomRoast())
		return embed, nil
	},
}
