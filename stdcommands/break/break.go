package _break

import (
	"fmt"
	"github.com/cirelion/flint/bot"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/common/scheduledevents2"
	"github.com/cirelion/flint/lib/dcmd"
	"time"
)

var Command = &commands.YAGCommand{
	Name:         "break",
	Description:  "Mutes you and hides all channels to help you focus. Default is 6 hours.",
	CmdCategory:  commands.CategoryGeneral,
	RequiredArgs: 0,
	Arguments: []*dcmd.ArgDef{
		{Name: "Duration", Type: &commands.DurationArg{}, Default: time.Hour * 6},
	},
	ApplicationCommandEnabled: true,
	RunFunc: func(data *dcmd.Data) (interface{}, error) {
		guildID := data.GuildData.GS.ID
		duration := data.Args[0].Value.(time.Duration)
		if duration.Hours() > 24 {
			return "Can't set a break for more than 24 hours!", nil
		}

		member, err := bot.GetMember(guildID, data.Author.ID)
		if err != nil {
			return nil, err
		}

		err = common.AddRoleDS(member, 1208188179249889320)
		if err != nil {
			return nil, err
		}

		err = scheduledevents2.ScheduleRemoveRole(data.Context(), data.GuildData.GS.ID, data.Author.ID, 1208188179249889320, time.Now().Add(duration))
		if err != nil {
			return nil, err
		}

		// cancel the event to add the role
		scheduledevents2.CancelAddRole(data.Context(), data.GuildData.GS.ID, data.Author.ID, 1208188179249889320)

		return fmt.Sprintf("Hope this helps you focus %s, see you in %s!", data.Author.Mention(), common.HumanizeDuration(common.DurationPrecisionMinutes, duration)), nil
	},
}
