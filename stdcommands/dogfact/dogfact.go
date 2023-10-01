package dogfact

import (
	"math/rand"

	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/lib/dcmd"
)

var Command = &commands.YAGCommand{
	CmdCategory:               commands.CategoryFun,
	Name:                      "DogFact",
	Aliases:                   []string{"dog", "dogfacts"},
	Description:               "Dog Facts",
	ApplicationCommandEnabled: true,
	DefaultEnabled:            true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		df := dogfacts[rand.Intn(len(dogfacts))]
		return df, nil
	},
}
