package moderation

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/botlabs-gg/yagpdb/v2/common"
	"github.com/botlabs-gg/yagpdb/v2/common/configstore"
	"github.com/botlabs-gg/yagpdb/v2/common/featureflags"
	"github.com/botlabs-gg/yagpdb/v2/common/pubsub"
	"github.com/lib/pq"
)

type Config struct {
	configstore.GuildConfigModel

	// Kick command
	KickEnabled          bool
	KickCmdRoles         pq.Int64Array `gorm:"type:bigint[]" valid:"role,true"`
	DeleteMessagesOnKick bool
	KickReasonOptional   bool
	KickMessage          string `valid:"template,5000"`

	// Ban
	BanEnabled           bool
	BanCmdRoles          pq.Int64Array `gorm:"type:bigint[]" valid:"role,true"`
	BanReasonOptional    bool
	BanMessage           string        `valid:"template,5000"`
	DefaultBanDeleteDays sql.NullInt64 `gorm:"default:1" valid:"0,7"`

	// Timeout
	TimeoutEnabled              bool
	TimeoutCmdRoles             pq.Int64Array `gorm:"type:bigint[]" valid:"role,true"`
	TimeoutReasonOptional       bool
	TimeoutRemoveReasonOptional bool
	TimeoutMessage              string        `valid:"template,5000"`
	DefaultTimeoutDuration      sql.NullInt64 `gorm:"default:10" valid:"1,40320"`

	// Mute/unmute
	MuteEnabled             bool
	MuteCmdRoles            pq.Int64Array `gorm:"type:bigint[]" valid:"role,true"`
	MuteRole                string        `valid:"role,true"`
	MuteDisallowReactionAdd bool
	MuteReasonOptional      bool
	UnmuteReasonOptional    bool
	MuteManageRole          bool
	MuteRemoveRoles         pq.Int64Array `gorm:"type:bigint[]" valid:"role,true"`
	MuteIgnoreChannels      pq.Int64Array `gorm:"type:bigint[]" valid:"channel,true"`
	MuteMessage             string        `valid:"template,5000"`
	UnmuteMessage           string        `valid:"template,5000"`
	DefaultMuteDuration     sql.NullInt64 `gorm:"default:10" valid:"0,"`

	// Warn
	WarnCommandsEnabled    bool
	WarnCmdRoles           pq.Int64Array `gorm:"type:bigint[]" valid:"role,true"`
	WarnIncludeChannelLogs bool
	WarnSendToModlog       bool
	WarnMessage            string `valid:"template,5000"`

	// Message logs
	EditLogChannel   string        `valid:"channel,true"`
	DeleteLogChannel string        `valid:"channel,true"`
	IgnoreChannels   pq.Int64Array `gorm:"type:bigint[]" valid:"channel,true"`
	IgnoreCategories pq.Int64Array `gorm:"type:bigint[]" valid:"channel,true"`

	//Star/Heart board
	StarBoardChannel  int64         `valid:"channel,true"`
	HeartBoardChannel int64         `valid:"channel,true"`
	ShowcaseChannels  pq.Int64Array `gorm:"type:bigint[]" valid:"channel,true"`

	//On Duty
	OnDutyRole                  string `valid:"role,true"`
	OnDutyChannelOne            string `valid:"channel,true"`
	OnDutyChannelOneDescription string `valid:"template,5000"`
	OnDutyChannelTwo            string `valid:"channel,true"`
	OnDutyChannelTwoDescription string `valid:"template,5000"`

	// Misc
	CleanEnabled     bool
	ReportEnabled    bool
	ActionChannel    string `valid:"channel,true"`
	ReportChannel    string `valid:"channel,true"`
	WatchListChannel string `valid:"channel,true"`
	ErrorChannel     string `valid:"channel,true"`
	LogUnbans        bool
	LogBans          bool
	LogKicks         bool `gorm:"default:true"`
	LogTimeouts      bool

	GiveRoleCmdEnabled bool
	GiveRoleCmdModlog  bool
	GiveRoleCmdRoles   pq.Int64Array `gorm:"type:bigint[]" valid:"role,true"`
}

func (c *Config) IntMuteRole() (r int64) {
	r, _ = strconv.ParseInt(c.MuteRole, 10, 64)
	return
}

func (c *Config) IntOnDutyRole() (r int64) {
	r, _ = strconv.ParseInt(c.OnDutyRole, 10, 64)
	return
}

func (c *Config) IntOnDutyChannelOne() (r int64) {
	r, _ = strconv.ParseInt(c.OnDutyChannelOne, 10, 64)
	return
}
func (c *Config) IntOnDutyChannelTwo() (r int64) {
	r, _ = strconv.ParseInt(c.OnDutyChannelTwo, 10, 64)
	return
}

func (c *Config) IntActionChannel() (r int64) {
	r, _ = strconv.ParseInt(c.ActionChannel, 10, 64)
	return
}

func (c *Config) IntEditLogChannel() (r int64) {
	r, _ = strconv.ParseInt(c.EditLogChannel, 10, 64)
	return
}

func (c *Config) IntDeleteLogChannel() (r int64) {
	r, _ = strconv.ParseInt(c.DeleteLogChannel, 10, 64)
	return
}

func (c *Config) IntReportChannel() (r int64) {
	r, _ = strconv.ParseInt(c.ReportChannel, 10, 64)
	return
}
func (c *Config) IntWatchListChannel() (r int64) {
	r, _ = strconv.ParseInt(c.WatchListChannel, 10, 64)
	return
}

func (c *Config) IntErrorChannel() (r int64) {
	r, _ = strconv.ParseInt(c.ErrorChannel, 10, 64)
	return
}

func (c *Config) GetName() string {
	return "moderation"
}

func (c *Config) TableName() string {
	return "moderation_configs"
}

func (c *Config) Save(guildID int64) error {
	c.GuildID = guildID
	err := configstore.SQL.SetGuildConfig(context.Background(), c)
	if err != nil {
		return err
	}

	if err = featureflags.UpdatePluginFeatureFlags(guildID, &Plugin{}); err != nil {
		return err
	}

	pubsub.Publish("mod_refresh_mute_override", guildID, nil)
	return err
}

type WarningModel struct {
	common.SmallModel
	GuildID  int64 `gorm:"index"`
	UserID   string
	AuthorID string

	// Username and discrim for author incase he/she leaves
	AuthorUsernameDiscrim string

	Message  string
	LogsLink string
}

func (w *WarningModel) TableName() string {
	return "moderation_warnings"
}

type ModLog struct {
	UserID  uint64 `gorm:"primary_key"`
	GuildID int64  `gorm:"index"`

	Warns []Warn `gorm:"foreignKey:ModLogID;references:UserID"`
	Mutes []Mute `gorm:"foreignKey:ModLogID;references:UserID"`
	Kicks []Kick `gorm:"foreignKey:ModLogID;references:UserID"`
	Bans  []Ban  `gorm:"foreignKey:ModLogID;references:UserID"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Warn struct {
	ID       uint `gorm:"primary_key"`
	ModLogID uint64
	AuthorID string

	Reason  string
	LogLink string
	Proof   string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Mute struct {
	ID       uint `gorm:"primary_key"`
	ModLogID uint64
	AuthorID string

	Reason   string
	Duration time.Duration
	LogLink  string
	Proof    string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Kick struct {
	ID       uint `gorm:"primary_key"`
	ModLogID uint64
	AuthorID string

	Reason  string
	LogLink string
	Proof   string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Ban struct {
	ID       uint `gorm:"primary_key"`
	ModLogID uint64
	AuthorID string

	Reason   string
	Duration time.Duration
	LogLink  string
	Proof    string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type WatchList struct {
	UserID   uint64 `gorm:"primary_key"`
	GuildID  int64  `gorm:"index"`
	AuthorID string

	MessageID         int64
	Reason            string
	HeadModeratorNote string
	Feuds             []Feud          `gorm:"foreignKey:WatchListID;references:UserID"`
	VerbalWarnings    []VerbalWarning `gorm:"foreignKey:WatchListID;references:UserID"`

	Ping         string
	LastPingedAt time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

type Feud struct {
	ID          uint  `gorm:"primary_key"`
	GuildID     int64 `gorm:"index"`
	AuthorID    int64
	WatchListID uint64

	FeudingUserName string
	Reason          string
	MessageLink     string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type VerbalWarning struct {
	ID          uint  `gorm:"primary_key"`
	GuildID     int64 `gorm:"index"`
	WatchListID uint64

	AuthorID    int64
	Reason      string
	MessageLink string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type MuteModel struct {
	common.SmallModel

	ExpiresAt time.Time

	GuildID int64 `gorm:"index"`
	UserID  int64

	AuthorID int64
	Reason   string

	RemovedRoles pq.Int64Array `gorm:"type:bigint[]"`
}

func (m *MuteModel) TableName() string {
	return "muted_users"
}

func (o OnDuty) TableName() string {
	return "on_duty"
}

type OnDuty struct {
	UserID  uint64 `gorm:"primary_key" json:"user_id"`
	GuildID int64  `gorm:"index" json:"guild_id"`

	OnDuty         bool
	OnDutyDuration time.Duration
	OnDutySetAt    time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}
