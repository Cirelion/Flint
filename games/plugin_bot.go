package games

import (
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
)

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Games",
		SysName:  "games",
		Category: common.PluginCategoryMisc,
	}
}

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})
	common.GORM.AutoMigrate(&Player{}, &Duel{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)

func (p *Plugin) BotInit() {
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		Screws,
		Fire,
		AcceptDuel,
		InitDuel,
		GiveScrews,
		ResetScrews,
	)
}
