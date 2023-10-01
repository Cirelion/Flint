package messagelogs

import (
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/common/configstore"
	"github.com/lib/pq"
)

type Plugin struct {
}

func (p *Plugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Message logging",
		SysName:  "message_logging",
		Category: common.PluginCategoryModeration,
	}
}

func RegisterPlugin() {
	plugin := &Plugin{}

	common.RegisterPlugin(plugin)

	configstore.RegisterConfig(configstore.SQL, &Config{})
	common.GORM.AutoMigrate(&Config{}, &Message{}, &Attachment{})
}

type Config struct {
	configstore.GuildConfigModel
	IgnoredChannels   pq.Int64Array `gorm:"type:bigint[]" valid:"channel,true"`
	IgnoredCategories pq.Int64Array `gorm:"type:bigint[]" valid:"channel,true"`
}

func (c *Config) GetName() string {
	return "messages"
}

func (c *Config) TableName() string {
	return "message_logs"
}
