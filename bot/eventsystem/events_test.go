package eventsystem

import (
	"testing"

	"github.com/cirelion/flint/common"

	"github.com/cirelion/flint/lib/discordgo"
)

type mockPlugin struct {
}

func (p *mockPlugin) PluginInfo() *common.PluginInfo {
	return &common.PluginInfo{
		Name:     "Mock",
		SysName:  "mock",
		Category: common.PluginCategoryCore,
	}
}

func TestAddHandlerOrder(t *testing.T) {
	firstTriggered := false
	h1 := func(evt *EventData) {
		firstTriggered = true
	}
	h2 := func(evt *EventData) {
		if !firstTriggered {
			t.Error("Unordered!")
		}
	}

	AddHandlerSecondLegacy(&mockPlugin{}, h2, EventReady)
	AddHandlerFirstLegacy(&mockPlugin{}, h1, EventReady)
	HandleEvent(nil, &discordgo.Ready{})
}
