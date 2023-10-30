package sleep

import (
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/common/scheduledevents2"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/moderation"
	"time"
)

var Command = &commands.YAGCommand{
	Name:         "sleep",
	Description:  "Mutes you so you can sleep in peace. Default is 6 hours.",
	CmdCategory:  commands.CategoryGeneral,
	RequiredArgs: 0,
	Arguments: []*dcmd.ArgDef{
		{Name: "Duration", Type: &commands.DurationArg{}, Default: time.Hour * 6},
	},
	ApplicationCommandEnabled: true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		guildID := data.GuildData.GS.ID
		config, err := moderation.GetConfig(guildID)
		duration := data.Args[0].Value.(time.Duration)
		if duration.Hours() > 24 {
			return "Can't set yourself to sleep for more than 24 hours!", nil
		}

		member, err := bot.GetMember(guildID, data.Author.ID)
		if err != nil {
			return nil, err
		}

		err = common.AddRoleDS(member, config.IntMuteRole())
		if err != nil {
			return nil, err
		}

		err = scheduledevents2.ScheduleRemoveRole(data.Context(), data.GuildData.GS.ID, data.Author.ID, config.IntMuteRole(), time.Now().Add(duration))
		if err != nil {
			return nil, err
		}

		// cancel the event to add the role
		scheduledevents2.CancelAddRole(data.Context(), data.GuildData.GS.ID, data.Author.ID, config.IntMuteRole())

		return fmt.Sprintf("Sweet dreams %s, see you in %s!", data.Author.Mention(), common.HumanizeDuration(common.DurationPrecisionMinutes, duration)), nil
	},
}
