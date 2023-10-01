package unbanserver

import (
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/stdcommands/util"
	"github.com/mediocregopher/radix/v3"
)

var Command = &commands.YAGCommand{
	Cooldown:             2,
	CmdCategory:          commands.CategoryDebug,
	HideFromCommandsPage: true,
	Name:                 "unbanserver",
	Description:          "Removes the bot ban from the specified server. Bot Owner Only",
	HideFromHelp:         true,
	RequiredArgs:         1,
	Arguments: []*dcmd.ArgDef{
		{Name: "server", Type: dcmd.String},
	},
	RunFunc: util.RequireOwner(func(data *dcmd.Data) (interface{}, error) {

		var unbanned bool
		err := common.RedisPool.Do(radix.Cmd(&unbanned, "SREM", "banned_servers", data.Args[0].Str()))
		if err != nil {
			return nil, err
		}

		if !unbanned {
			return "Server wasn't banned", nil
		}

		return "Unbanned server", nil
	}),
}
