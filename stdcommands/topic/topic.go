package topic

import (
	"context"
	"fmt"
	"github.com/cirelion/flint/fun"
	"math/rand"
	"strings"
	"time"

	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/lib/dcmd"
)

var Topic = &commands.YAGCommand{
	Cooldown:                  5,
	CmdCategory:               commands.CategoryFun,
	Name:                      "Topic",
	Description:               "Generates a conversation topic to help chat get moving.",
	DefaultEnabled:            true,
	ApplicationCommandEnabled: true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		config, err := fun.GetConfig(context.Background(), data.GuildData.GS.ID)
		if err != nil {
			return false, err
		}

		topics := strings.Split(config.Topics, "\n")
		topic := strings.TrimSpace(topics[r.Intn(len(topics))])

		return fmt.Sprintf("> %s", topic), nil
	},
}

var NSFWTopic = &commands.YAGCommand{
	Cooldown:                  5,
	CmdCategory:               commands.CategoryFun,
	Name:                      "NSFWTopic",
	Description:               "Generates a NSFW conversation topic to help chat get moving.",
	DefaultEnabled:            true,
	NSFW:                      true,
	ApplicationCommandEnabled: true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		config, err := fun.GetConfig(context.Background(), data.GuildData.GS.ID)
		if err != nil {
			return false, err
		}

		topics := strings.Split(config.NSFWTopics, "\n")
		topic := strings.TrimSpace(topics[r.Intn(len(topics))])

		return fmt.Sprintf("> %s", topic), nil
	},
}
