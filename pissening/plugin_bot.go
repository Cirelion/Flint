package pissening

import (
	"context"
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/bot/eventsystem"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/fun"
	"github.com/cirelion/flint/fun/models"
	"github.com/cirelion/flint/lib/discordgo"
	"math/rand"
	"time"
)

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Pissening",
		SysName:  "pissening",
		Category: common.PluginCategoryMisc,
	}
}

func RegisterPlugin() {
	common.RegisterPlugin(&Plugin{})
	common.GORM.AutoMigrate(&Pissening{}, &Pisser{})
}

var _ bot.BotInitHandler = (*Plugin)(nil)
var logger = common.GetPluginLogger(&Plugin{})

func (p *Plugin) BotInit() {
	eventsystem.AddHandlerAsyncLastLegacy(p, handleInteractionCreate, eventsystem.EventInteractionCreate)
}

func handleInteractionCreate(evt *eventsystem.EventData) {
	ic := evt.InteractionCreate()
	if ic.Type != discordgo.InteractionMessageComponent {
		return
	}
	if ic.GuildID == 0 {
		//DM interactions are handled via pubsub
		return
	}

	customID := ic.MessageComponentData().CustomID

	switch customID {
	case JoinID:
		handleJoinPissening(evt.Context(), ic)
	case StartID:
		handleStartPissening(evt.Context(), ic)
	case PissID:
		handlePissPissening(evt.Context(), ic)
	case StopID:
		handleStopPissening(evt.Context(), ic)
	}
}

func handleStopPissening(ctx context.Context, ic *discordgo.InteractionCreate) {
	config, err := fun.GetConfig(ctx, ic.GuildID)
	if err != nil {
		logger.Error(err)
		return
	}

	pissening := &Pissening{
		GuildID: ic.GuildID,
	}

	err = common.GORM.Model(pissening).Where("author_id = ?", ic.Member.User.ID).Where("pissening_state != ?", Ended).First(pissening).Error
	if err == nil {
		pissening.PisseningState = Ended
		pissening.MessageID = handleMessage(config.PChannel, pissening, true)

		common.GORM.Model(pissening).Update(pissening)
	}

	common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
}

func handlePissPissening(ctx context.Context, ic *discordgo.InteractionCreate) {
	config, err := fun.GetConfig(ctx, ic.GuildID)
	if err != nil {
		logger.Error(err)
		return
	}

	pisser := getPisser(ic.GuildID, ic.Member.User.ID)
	pissening := &Pissening{
		GuildID: ic.GuildID,
	}

	err = common.GORM.Model(pissening).Where("pissening_state = ?", Started).First(pissening).Error
	handlePiss(config, pissening, pisser, ic.Member.User)

	common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
}

func handleStartPissening(ctx context.Context, ic *discordgo.InteractionCreate) {
	config, err := fun.GetConfig(ctx, ic.GuildID)
	if err != nil {
		logger.Error(err)
		return
	}

	pissening := &Pissening{
		GuildID: ic.GuildID,
	}
	previousPisser := &Pisser{
		GuildID: ic.GuildID,
	}
	err = common.GORM.Model(previousPisser).Where("has_p_role = true").Find(previousPisser).Error
	if err == nil {
		common.BotSession.GuildMemberRoleRemove(previousPisser.GuildID, previousPisser.UserID, config.PRole)
	}

	err = common.GORM.Model(pissening).Where("pissening_state != ?", Ended).First(pissening).Error
	if err == nil {
		if pissening.AuthorID != ic.Member.User.ID {
			return
		}

		pissening.PisseningState = Started
		pissening.MessageID = handleMessage(config.PChannel, pissening, true)

		common.GORM.Model(pissening).Update(pissening)
	}

	common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
}
func handleJoinPissening(ctx context.Context, ic *discordgo.InteractionCreate) {
	userID := ic.Member.User.ID
	config, err := fun.GetConfig(ctx, ic.GuildID)
	if err != nil {
		logger.Error(err)
		return
	}

	pissening := &Pissening{GuildID: ic.GuildID}
	err = common.GORM.Model(pissening).Where("pissening_state = ?", Initialized).First(pissening).Error
	if err == nil {
		if common.ContainsInt64Slice(pissening.Pissers, userID) {
			return
		}

		pissening.Pissers = append(pissening.Pissers, userID)
		pissening.MessageID = handleMessage(config.PChannel, pissening, false)
		common.GORM.Model(pissening).Update(pissening)

		pisser := getPisser(ic.GuildID, userID)
		pisser.CompetedPissenings++
		pisser.ActivePissening = pissening
		common.GORM.Model(pisser).Update(pisser)
	}

	common.BotSession.CreateInteractionResponse(ic.ID, ic.Token, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
}

func handlePiss(config *models.FunSetting, pissening *Pissening, pisser Pisser, author *discordgo.User) {
	limit := len(pissening.Pissers) * 5
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomIndex := r.Intn(len(pissReplies))

	if !common.ContainsInt64Slice(pissening.Pissers, pisser.UserID) {
		msg, _ := common.BotSession.ChannelMessageSend(config.PChannel, fmt.Sprintf("%s pissed wildly around him, try not to slip!", author.Mention()))
		go func() {
			time.Sleep(30 * time.Second)
			common.BotSession.ChannelMessageDelete(config.PChannel, msg.ID)
		}()

		return
	}

	pissening.Pisses = append(pissening.Pisses, pisser.UserID)
	pisser.TotalPisses += 1

	if len(pissening.Pisses) <= limit {
		msg, _ := common.BotSession.ChannelMessageSend(config.PChannel, fmt.Sprintf(pissReplies[randomIndex], author.Mention()))
		if limit-len(pissening.Pisses) == 5 {
			handleMessage(config.PChannel, pissening, true)
		}

		if len(pissening.Pisses) == limit {
			bot.SendMessage(pissening.GuildID, config.PChannel, "❌❌ GUYS! We are on ⚠️ PISS RESTRICTIONS! ⚠️ There will be absolutely 🚫 NO MORE PISSING ALLOWED!!!! 🚫 Spread The Word! Do Not Piss! ❌❌")
		}

		go func() {
			time.Sleep(30 * time.Second)
			common.BotSession.ChannelMessageDelete(config.PChannel, msg.ID)
		}()
	}

	if len(pissening.Pisses) > limit {
		randomIndex = r.Intn(len(overflowReplies))

		pissening.JarSpiller = pisser.UserID
		pissening.PisseningState = Ended
		pissening.MessageID = handleMessage(config.PChannel, pissening, true)

		pisser.JarsSpilled += 1
		pisser.HasPRole = common.BoolToPointer(true)
		common.BotSession.GuildMemberRoleAdd(pisser.GuildID, pisser.UserID, config.PRole)

		common.BotSession.ChannelMessageSend(config.PChannel, fmt.Sprintf(overflowReplies[randomIndex], author.Mention()))
	}

	common.GORM.Model(pissening).Update(pissening)
	common.GORM.Model(pisser).Update(pisser)
}

func (p *Plugin) AddCommands() {
	commands.AddRootCommands(p,
		Pisstory,
		Piss,
		EnterPissening,
		StartPissening,
		StopPissening,
	)
}
