package fun

import (
	_ "embed"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/common/cplogs"
	"github.com/cirelion/flint/common/featureflags"
	"github.com/cirelion/flint/fun/models"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/web"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"goji.io"
	"goji.io/pat"
	"net/http"
)

//go:embed assets/commands_settings.html
var PageHTMLSettings string

type FunConfigForm struct {
	Topics     string
	NSFWTopics string
	DadJokes   string
	Wyrs       string
	NSFWWyrs   string
	PChannel   int64
	PRole      int64
}

var (
	panelLogKeyUpdatedSettings = cplogs.RegisterActionFormat(&cplogs.ActionFormat{Key: "fun_settings_updated", FormatString: "Update fun settings"})
)

func (p FunConfigForm) FunSetting() *models.FunSetting {
	return &models.FunSetting{
		Topics:     p.Topics,
		NSFWTopics: p.NSFWTopics,
		DadJokes:   p.DadJokes,
		Wyrs:       p.Wyrs,
		NSFWWyrs:   p.NSFWWyrs,
		PChannel:   p.PChannel,
		PRole:      p.PRole,
	}
}

func (p *Plugin) InitWeb() {
	web.AddHTMLTemplate("fun/assets/commands_settings.html", PageHTMLSettings)
	web.AddSidebarItem(web.SidebarCategoryFun, &web.SidebarItem{
		Name: "Commands",
		URL:  "fun_settings",
		Icon: "fas fa-terminal",
	})

	subMux := goji.SubMux()

	web.CPMux.Handle(pat.New("/fun_settings"), subMux)
	web.CPMux.Handle(pat.New("/fun_settings/*"), subMux)

	mainGetHandler := web.RenderHandler(HandleGetFunSettings, "cp_fun_settings")

	subMux.Handle(pat.Get(""), mainGetHandler)
	subMux.Handle(pat.Get("/"), mainGetHandler)

	subMux.Handle(pat.Post(""), web.ControllerPostHandler(HandlePostFunSettings, mainGetHandler, FunConfigForm{}))
	subMux.Handle(pat.Post("/"), web.ControllerPostHandler(HandlePostFunSettings, mainGetHandler, FunConfigForm{}))
}

func HandleGetFunSettings(w http.ResponseWriter, r *http.Request) interface{} {
	activeGuild, templateData := web.GetBaseCPContextData(r.Context())

	if _, ok := templateData["FunSettings"]; !ok {
		settings, err := GetConfig(r.Context(), activeGuild.ID)
		if !web.CheckErr(templateData, err, "Failed retrieving settings", web.CtxLogger(r.Context()).Error) {
			templateData["FunSettings"] = settings
		}
	}

	return templateData
}

func HandlePostFunSettings(w http.ResponseWriter, r *http.Request) (templateData web.TemplateData, err error) {
	activeGuild, templateData := web.GetBaseCPContextData(r.Context())
	templateData["VisibleURL"] = "/manage/" + discordgo.StrID(activeGuild.ID) + "/reputation"

	form := r.Context().Value(common.ContextKeyParsedForm).(*FunConfigForm)
	conf := form.FunSetting()
	conf.GuildID = activeGuild.ID

	templateData["RepSettings"] = conf

	err = conf.UpsertG(r.Context(), true, []string{"guild_id"}, boil.Whitelist(
		"topics",
		"nsfw_topics",
		"dad_jokes",
		"wyrs",
		"nsfw_wyrs",
		"p_channel",
		"p_role",
	), boil.Infer())

	if err == nil {
		featureflags.MarkGuildDirty(activeGuild.ID)
		go cplogs.RetryAddEntry(web.NewLogEntryFromContext(r.Context(), panelLogKeyUpdatedSettings))
	}

	return
}
