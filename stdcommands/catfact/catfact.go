package catfact

import (
	"math/rand"

	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/lib/dcmd"
)

var Command = &commands.YAGCommand{
	CmdCategory:               commands.CategoryFun,
	Name:                      "CatFact",
	Aliases:                   []string{"cf", "cat", "catfacts"},
	Description:               "Cat Facts",
	DefaultEnabled:            true,
	ApplicationCommandEnabled: true,

	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		cf := Catfacts[rand.Intn(len(Catfacts))]
		return cf, nil
	},
}
