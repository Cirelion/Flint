package main

import (
	"github.com/botlabs-gg/yagpdb/v2/analytics"
	"github.com/botlabs-gg/yagpdb/v2/antiphishing"
	"github.com/botlabs-gg/yagpdb/v2/applications"
	"github.com/botlabs-gg/yagpdb/v2/autorole"
	"github.com/botlabs-gg/yagpdb/v2/common/featureflags"
	"github.com/botlabs-gg/yagpdb/v2/common/prom"
	"github.com/botlabs-gg/yagpdb/v2/common/run"
	"github.com/botlabs-gg/yagpdb/v2/games"
	"github.com/botlabs-gg/yagpdb/v2/giveaways"
	"github.com/botlabs-gg/yagpdb/v2/heartboard"
	"github.com/botlabs-gg/yagpdb/v2/lib/confusables"
	"github.com/botlabs-gg/yagpdb/v2/messagelogs"
	"github.com/botlabs-gg/yagpdb/v2/polls"
	"github.com/botlabs-gg/yagpdb/v2/web/discorddata"

	// Core yagpdb packages
	"github.com/botlabs-gg/yagpdb/v2/admin"
	"github.com/botlabs-gg/yagpdb/v2/bot/paginatedmessages"
	"github.com/botlabs-gg/yagpdb/v2/common/internalapi"
	"github.com/botlabs-gg/yagpdb/v2/common/scheduledevents2"

	// Plugin imports
	"github.com/botlabs-gg/yagpdb/v2/automod"

	"github.com/botlabs-gg/yagpdb/v2/commands"
	"github.com/botlabs-gg/yagpdb/v2/customcommands"
	"github.com/botlabs-gg/yagpdb/v2/discordlogger"
	"github.com/botlabs-gg/yagpdb/v2/logs"
	"github.com/botlabs-gg/yagpdb/v2/moderation"
	"github.com/botlabs-gg/yagpdb/v2/notifications"
	"github.com/botlabs-gg/yagpdb/v2/premium"
	"github.com/botlabs-gg/yagpdb/v2/premium/patreonpremiumsource"
	"github.com/botlabs-gg/yagpdb/v2/reputation"
	"github.com/botlabs-gg/yagpdb/v2/safebrowsing"
	"github.com/botlabs-gg/yagpdb/v2/serverstats"
	"github.com/botlabs-gg/yagpdb/v2/stdcommands"
	//"github.com/botlabs-gg/yagpdb/v2/tickets"
	"github.com/botlabs-gg/yagpdb/v2/verification"
	// External plugins
)

func main() {

	run.Init()

	//BotSession.LogLevel = discordgo.LogInformational
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
	//tickets.RegisterPlugin()
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
