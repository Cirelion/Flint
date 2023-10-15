package main

import (
	"github.com/cirelion/flint/analytics"
	"github.com/cirelion/flint/antiphishing"
	"github.com/cirelion/flint/applications"
	"github.com/cirelion/flint/autorole"
	"github.com/cirelion/flint/common/featureflags"
	"github.com/cirelion/flint/common/prom"
	"github.com/cirelion/flint/common/run"
	"github.com/cirelion/flint/fun"
	"github.com/cirelion/flint/games"
	"github.com/cirelion/flint/giveaways"
	"github.com/cirelion/flint/heartboard"
	"github.com/cirelion/flint/lib/confusables"
	"github.com/cirelion/flint/messagelogs"
	"github.com/cirelion/flint/polls"
	"github.com/cirelion/flint/reddit"
	"github.com/cirelion/flint/tickets"
	"github.com/cirelion/flint/web/discorddata"

	// Core yagpdb packages
	"github.com/cirelion/flint/admin"
	"github.com/cirelion/flint/bot/paginatedmessages"
	"github.com/cirelion/flint/common/internalapi"
	"github.com/cirelion/flint/common/scheduledevents2"

	// Plugin imports
	"github.com/cirelion/flint/automod"

	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/customcommands"
	"github.com/cirelion/flint/discordlogger"
	"github.com/cirelion/flint/logs"
	"github.com/cirelion/flint/moderation"
	"github.com/cirelion/flint/notifications"
	"github.com/cirelion/flint/premium"
	"github.com/cirelion/flint/premium/patreonpremiumsource"
	"github.com/cirelion/flint/reputation"
	"github.com/cirelion/flint/safebrowsing"
	"github.com/cirelion/flint/serverstats"
	"github.com/cirelion/flint/stdcommands"
	//"github.com/cirelion/flint/tickets"
	"github.com/cirelion/flint/verification"
	// External plugins
)

func main() {

	run.Init()

	//BotSession.LogLevel = discordgo.LogInformational
	fun.RegisterPlugin()
	applications.RegisterPlugin()
	polls.RegisterPlugin()
	giveaways.RegisterPlugin()
	games.RegisterPlugin()
	heartboard.RegisterPlugin()
	paginatedmessages.RegisterPlugin()
	discorddata.RegisterPlugin()

	// Setup plugins
	analytics.RegisterPlugin()
	safebrowsing.RegisterPlugin()
	antiphishing.RegisterPlugin()
	discordlogger.Register()
	commands.RegisterPlugin()
	stdcommands.RegisterPlugin()
	serverstats.RegisterPlugin()
	notifications.RegisterPlugin()
	customcommands.RegisterPlugin()
	moderation.RegisterPlugin()
	reputation.RegisterPlugin()
	automod.RegisterPlugin()
	logs.RegisterPlugin()
	messagelogs.RegisterPlugin()
	autorole.RegisterPlugin()
	reddit.RegisterPlugin()
	tickets.RegisterPlugin()
	verification.RegisterPlugin()
	premium.RegisterPlugin()
	patreonpremiumsource.RegisterPlugin()
	scheduledevents2.RegisterPlugin()
	admin.RegisterPlugin()
	internalapi.RegisterPlugin()
	prom.RegisterPlugin()
	featureflags.RegisterPlugin()

	// Register confusables replacer
	confusables.Init()

	run.Run()
}
