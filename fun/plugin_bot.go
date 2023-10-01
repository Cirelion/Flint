package fun

import (
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
)

var _ bot.BotInitHandler = (*Plugin)(nil)
var _ commands.CommandProvider = (*Plugin)(nil)

func (p *Plugin) AddCommands() {
	//commands.AddRootCommands(p, cmds...)
}

func (p *Plugin) BotInit() {
}
