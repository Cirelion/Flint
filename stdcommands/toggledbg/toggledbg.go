package toggledbg

import (
	"github.com/cirelion/flint/common"
	"github.com/sirupsen/logrus"

	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/stdcommands/util"
)

var Command = &commands.YAGCommand{
	CmdCategory:          commands.CategoryDebug,
	HideFromCommandsPage: true,
	Name:                 "toggledbg",
	Description:          "Toggles Debug Logging. Restarting the bot will always reset debug logging. Bot Owner Only",
	HideFromHelp:         true,
	RunFunc: util.RequireOwner(func(data *dcmd.Data) (interface{}, error) {
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			common.SetLoggingLevel(logrus.InfoLevel)
			return "Disabled debug logging", nil
		}

		common.SetLoggingLevel(logrus.DebugLevel)
		return "Enabled debug logging", nil

	}),
}
