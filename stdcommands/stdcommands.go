package stdcommands

import (
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/bot/eventsystem"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/stdcommands/viewperms"

	//"github.com/cirelion/flint/stdcommands/advice"
	"github.com/cirelion/flint/stdcommands/allocstat"
	"github.com/cirelion/flint/stdcommands/banserver"
	"github.com/cirelion/flint/stdcommands/calc"
	"github.com/cirelion/flint/stdcommands/catfact"
	"github.com/cirelion/flint/stdcommands/ccreqs"
	"github.com/cirelion/flint/stdcommands/createinvite"
	"github.com/cirelion/flint/stdcommands/currentshard"
	"github.com/cirelion/flint/stdcommands/currenttime"
	"github.com/cirelion/flint/stdcommands/customembed"
	"github.com/cirelion/flint/stdcommands/dadjoke"
	"github.com/cirelion/flint/stdcommands/dcallvoice"
	//"github.com/cirelion/flint/stdcommands/define"
	//"github.com/cirelion/flint/stdcommands/dictionary"
	"github.com/cirelion/flint/stdcommands/dogfact"
	"github.com/cirelion/flint/stdcommands/eightball"
	"github.com/cirelion/flint/stdcommands/findserver"
	"github.com/cirelion/flint/stdcommands/forex"
	"github.com/cirelion/flint/stdcommands/globalrl"
	"github.com/cirelion/flint/stdcommands/guildunavailable"
	"github.com/cirelion/flint/stdcommands/howlongtobeat"
	"github.com/cirelion/flint/stdcommands/info"
	//"github.com/cirelion/flint/stdcommands/inspire"
	"github.com/cirelion/flint/stdcommands/invite"
	"github.com/cirelion/flint/stdcommands/leaveserver"
	"github.com/cirelion/flint/stdcommands/listflags"
	"github.com/cirelion/flint/stdcommands/listroles"
	"github.com/cirelion/flint/stdcommands/memstats"
	"github.com/cirelion/flint/stdcommands/ping"
	//"github.com/cirelion/flint/stdcommands/roast"
	"github.com/cirelion/flint/stdcommands/roll"
	"github.com/cirelion/flint/stdcommands/setstatus"
	"github.com/cirelion/flint/stdcommands/simpleembed"
	"github.com/cirelion/flint/stdcommands/sleep"
	"github.com/cirelion/flint/stdcommands/statedbg"
	"github.com/cirelion/flint/stdcommands/stateinfo"
	"github.com/cirelion/flint/stdcommands/throw"
	"github.com/cirelion/flint/stdcommands/toggledbg"
	"github.com/cirelion/flint/stdcommands/topcommands"
	"github.com/cirelion/flint/stdcommands/topevents"
	//"github.com/cirelion/flint/stdcommands/topgames"
	"github.com/cirelion/flint/stdcommands/topic"
	//"github.com/cirelion/flint/stdcommands/topservers"
	"github.com/cirelion/flint/stdcommands/unbanserver"
	"github.com/cirelion/flint/stdcommands/undelete"
	//"github.com/cirelion/flint/stdcommands/weather"
	"github.com/cirelion/flint/stdcommands/wouldyourather"
	//"github.com/cirelion/flint/stdcommands/xkcd"
	"github.com/cirelion/flint/stdcommands/yagstatus"
)

var (
	_ bot.BotInitHandler       = (*Plugin)(nil)
	_ commands.CommandProvider = (*Plugin)(nil)
)

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Standard Commands",
		SysName:  "standard_commands",
		Category: common.PluginCategoryCore,
	}
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		// Info
		info.Command,
		invite.Command,

		// Standard
		//define.Command,
		//weather.Command,
		calc.Command,
		topic.Topic,
		topic.NSFWTopic,
		catfact.Command,
		dadjoke.Command,
		dogfact.Command,
		//advice.Command,
		ping.Command,
		throw.Command,
		roll.Command,
		customembed.Command,
		simpleembed.Command,
		currenttime.Command,
		listroles.Command,
		memstats.Command,
		wouldyourather.Wyr,
		wouldyourather.NSFWWyr,
		undelete.Command,
		viewperms.Command,
		//topgames.Command,
		//xkcd.Command,
		howlongtobeat.Command,
		//inspire.Command,
		forex.Command,
		//roast.Command,
		eightball.Command,

		// Maintenance
		stateinfo.Command,
		leaveserver.Command,
		banserver.Command,
		allocstat.Command,
		unbanserver.Command,
		//topservers.Command,
		topcommands.Command,
		topevents.Command,
		currentshard.Command,
		guildunavailable.Command,
		yagstatus.Command,
		setstatus.Command,
		createinvite.Command,
		findserver.Command,
		dcallvoice.Command,
		ccreqs.Command,
		sleep.Command,
		toggledbg.Command,
		globalrl.Command,
		listflags.Command,
	)

	statedbg.Commands()
	//commands.AddRootCommands(p, dictionary.Command)
}

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, ping.HandleMessageCreate, eventsystem.EventMessageCreate)
}

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})
}
