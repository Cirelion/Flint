package fun

//go:generate sqlboiler --no-hooks psql

import (
	"context"
	"database/sql"
	"emperror.dev/errors"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/fun/models"
)

var logger = common.GetPluginLogger(&Plugin{})

func RegisterPlugin() {
	plugin := &Plugin{}

	common.InitSchemas("fun", DBSchemas...)

	common.RegisterPlugin(plugin)
}

type Plugin struct{}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Fun commands",
		SysName:  "fun",
		Category: common.PluginCategoryMisc,
	}
}

func DefaultConfig(guildID int64) *models.FunSetting {
	return &models.FunSetting{
		GuildID: guildID,
	}
}

func GetConfig(ctx context.Context, guildID int64) (*models.FunSetting, error) {
	conf, err := models.FindFunSettingG(ctx, guildID)
	if err != nil {
		if err == sql.ErrNoRows {
			return DefaultConfig(guildID), nil
		}
		return nil, errors.WrapIf(err, "Fun.GetConfig")
	}

	return conf, nil
}
