package tickets

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cirelion/flint/analytics"
	"github.com/cirelion/flint/commands"
	"github.com/cirelion/flint/common"
	"github.com/cirelion/flint/lib/dcmd"
	"github.com/cirelion/flint/lib/discordgo"
	"github.com/cirelion/flint/lib/dstate"
	"github.com/cirelion/flint/tickets/models"
	"github.com/cirelion/flint/web"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

const InTicketPerms = discordgo.PermissionReadMessageHistory | discordgo.PermissionReadMessages | discordgo.PermissionSendMessages | discordgo.PermissionEmbedLinks | discordgo.PermissionAttachFiles

var _ commands.CommandProvider = (*Plugin)(nil)

func createTicketsDisabledError(guild *dcmd.GuildContextData) string {
	return fmt.Sprintf("**The tickets system is disabled for this server.** Enable it at: <%s/tickets/settings>.", web.ManageServerURL(guild))
}

func (p *Plugin) AddCommands() {

	categoryTickets := &dcmd.Category{
		Name:        "Tickets",
		Description: "Ticket commands",
		HelpEmoji:   "🎫",
		EmbedColor:  0x42b9f4,
	}

	cmdAddParticipant := &commands.YAGCommand{
		CmdCategory:  categoryTickets,
		Name:         "AddUser",
		Description:  "Adds a user to the ticket in this channel",
		RequiredArgs: 1,
		Arguments: []*dcmd.ArgDef{
			{Name: "target", Type: &commands.MemberArg{}},
		},

		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			target := parsed.Args[0].Value.(*dstate.MemberState)

			currentTicket := parsed.Context().Value(CtxKeyCurrentTicket).(*Ticket)

		OUTER:
			for _, v := range parsed.GuildData.CS.PermissionOverwrites {
				if v.Type == discordgo.PermissionOverwriteTypeMember && v.ID == target.User.ID {
					if (v.Allow & InTicketPerms) == InTicketPerms {
						return "User is already part of the ticket", nil
					}

					break OUTER
				}
			}

			err := common.BotSession.ChannelPermissionSet(currentTicket.Ticket.ChannelID, target.User.ID, discordgo.PermissionOverwriteTypeMember, InTicketPerms, 0)
			if err != nil {
				return nil, err
			}

			return fmt.Sprintf("Added %s to the ticket", target.User.String()), nil
		},
	}

	//cmdRemoveParticipant := &commands.YAGCommand{
	//	CmdCategory:  categoryTickets,
	//	Name:         "RemoveUser",
	//	Description:  "Removes a user from the ticket",
	//	RequiredArgs: 1,
	//	Arguments: []*dcmd.ArgDef{
	//		{Name: "target", Type: &commands.MemberArg{}},
	//	},
	//
	//	RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
	//		target := parsed.Args[0].Value.(*dstate.MemberState)
	//
	//		currentTicket := parsed.Context().Value(CtxKeyCurrentTicket).(*Ticket)
	//
	//		foundUser := false
	//
	//	OUTER:
	//		for _, v := range parsed.GuildData.CS.PermissionOverwrites {
	//			if v.Type == discordgo.PermissionOverwriteTypeMember && v.ID == target.User.ID {
	//				if (v.Allow & InTicketPerms) == InTicketPerms {
	//					foundUser = true
	//				}
	//
	//				break OUTER
	//			}
	//		}
	//
	//		if !foundUser {
	//			return fmt.Sprintf("%s is already not (explicitly) part of this ticket", target.User.String()), nil
	//		}
	//
	//		err := common.BotSession.ChannelPermissionDelete(currentTicket.Ticket.ChannelID, target.User.ID)
	//		if err != nil {
	//			return nil, err
	//		}
	//
	//		return fmt.Sprintf("Removed %s from the ticket", target.User.String()), nil
	//	},
	//}

	closingTickets := make(map[int64]bool)
	var closingTicketsLock sync.Mutex

	cmdCloseTicket := &commands.YAGCommand{
		CmdCategory: categoryTickets,
		Name:        "Close",
		Aliases:     []string{"end", "delete"},
		Description: "Closes the ticket",
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			conf := parsed.Context().Value(CtxKeyConfig).(*models.TicketConfig)
			currentTicket := parsed.Context().Value(CtxKeyCurrentTicket).(*Ticket)

			// protect against calling close multiple times at the same time
			closingTicketsLock.Lock()
			if _, ok := closingTickets[currentTicket.Ticket.ChannelID]; ok {
				closingTicketsLock.Unlock()
				return "Already working on closing this ticket, please wait...", nil
			}
			closingTickets[currentTicket.Ticket.ChannelID] = true
			closingTicketsLock.Unlock()
			defer func() {
				closingTicketsLock.Lock()
				delete(closingTickets, currentTicket.Ticket.ChannelID)
				closingTicketsLock.Unlock()
			}()

			// send a heads up that this can take a while
			common.BotSession.ChannelMessageSend(parsed.ChannelID, "Closing ticket, creating logs, downloading attachments and so on.\nThis may take a while if the ticket is big.")

			currentTicket.Ticket.ClosedAt.Time = time.Now()
			currentTicket.Ticket.ClosedAt.Valid = true

			isAdminsOnly := ticketIsAdminOnly(conf, parsed.GuildData.CS)

			// create the logs, download the attachments
			err := createLogs(parsed, conf, currentTicket.Ticket, isAdminsOnly, &discordgo.MessageEmbed{
				URL:         fmt.Sprintf("%s/manage/%d/tickets/%d", web.BaseURL(), parsed.GuildData.GS.ID, currentTicket.Ticket.LocalID),
				Title:       fmt.Sprintf("Ticket #%d - '%s' closed", currentTicket.Ticket.LocalID, currentTicket.Ticket.Title),
				Description: fmt.Sprintf("Author: %s", currentTicket.Ticket.AuthorUsernameDiscrim),
				Color:       0xf23c3c,
			})
			if err != nil {
				return nil, err
			}

			// if everything went well, delete the channel
			_, err = common.BotSession.ChannelDelete(currentTicket.Ticket.ChannelID)
			if err != nil {
				return nil, err
			}

			_, err = currentTicket.Ticket.UpdateG(parsed.Context(), boil.Whitelist("closed_at"))
			if err != nil {
				return nil, err
			}

			return "", nil
		},
	}

	//cmdAdminsOnly := &commands.YAGCommand{
	//	CmdCategory: categoryTickets,
	//	Name:        "AdminsOnly",
	//	Aliases:     []string{"adminonly", "ao"},
	//	Description: "Toggle admins only mode for this ticket",
	//	RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
	//
	//		conf := parsed.Context().Value(CtxKeyConfig).(*models.TicketConfig)
	//
	//		isAdminsOnlyCurrently := true
	//
	//		modOverwrites := make([]discordgo.PermissionOverwrite, 0)
	//
	//		for _, ow := range parsed.GuildData.CS.PermissionOverwrites {
	//			if ow.Type == discordgo.PermissionOverwriteTypeRole && common.ContainsInt64Slice(conf.ModRoles, ow.ID) {
	//				if (ow.Allow & InTicketPerms) == InTicketPerms {
	//					// one of the mod roles has ticket perms, this is not a admin ticket currently
	//					isAdminsOnlyCurrently = false
	//				}
	//
	//				modOverwrites = append(modOverwrites, ow)
	//			}
	//		}
	//
	//		// update existing overwrites
	//		for _, v := range modOverwrites {
	//			var err error
	//			if isAdminsOnlyCurrently {
	//				// add back the mods to this ticket
	//				if (v.Allow & InTicketPerms) != InTicketPerms {
	//					// add it back to allows, remove from denies
	//					newAllows := v.Allow | InTicketPerms
	//					newDenies := v.Deny & (^InTicketPerms)
	//					err = common.BotSession.ChannelPermissionSet(parsed.ChannelID, v.ID, discordgo.PermissionOverwriteTypeRole, newAllows, newDenies)
	//				}
	//			} else {
	//				// remove the mods from this ticket
	//				if (v.Allow & InTicketPerms) == InTicketPerms {
	//					// remove it from allows
	//					newAllows := v.Allow & (^InTicketPerms)
	//					err = common.BotSession.ChannelPermissionSet(parsed.ChannelID, v.ID, discordgo.PermissionOverwriteTypeRole, newAllows, v.Deny)
	//				}
	//			}
	//
	//			if err != nil {
	//				logger.WithError(err).WithField("guild", parsed.GuildData.GS.ID).Error("[tickets] failed to update channel overwrite")
	//			}
	//		}
	//
	//		if isAdminsOnlyCurrently {
	//			// add the missing overwrites for the missing roles
	//		OUTER:
	//			for _, v := range conf.ModRoles {
	//				for _, ow := range modOverwrites {
	//					if ow.ID == v {
	//						// already handled above
	//						continue OUTER
	//					}
	//				}
	//
	//				// need to create a new overwrite
	//				err := common.BotSession.ChannelPermissionSet(parsed.ChannelID, v, discordgo.PermissionOverwriteTypeRole, InTicketPerms, 0)
	//				if err != nil {
	//					logger.WithError(err).WithField("guild", parsed.GuildData.GS.ID).Error("[tickets] failed to create channel overwrite")
	//				}
	//			}
	//		}
	//
	//		if isAdminsOnlyCurrently {
	//			return "Added back mods to the ticket", nil
	//		}
	//
	//		return "Removed mods from this ticket", nil
	//	},
	//}

	container, _ := commands.CommandSystem.Root.Sub("tickets", "ticket")
	container.Description = "Command to manage the ticket system"
	container.NotFound = commands.CommonContainerNotFoundHandler(container, "")
	container.AddMidlewares(
		func(inner dcmd.RunFunc) dcmd.RunFunc {
			return func(data *dcmd.Data) (interface{}, error) {

				conf, err := models.FindTicketConfigG(data.Context(), data.GuildData.GS.ID)
				if err != nil {
					if err != sql.ErrNoRows {
						return nil, err
					}

					conf = &models.TicketConfig{}
				}

				if conf.Enabled {
					go analytics.RecordActiveUnit(data.GuildData.GS.ID, &Plugin{}, "cmd_used")
				}

				activeTicket, err := models.Tickets(qm.Where("channel_id = ? AND guild_id = ?", data.GuildData.CS.ID, data.GuildData.GS.ID)).OneG(data.Context())
				if err != nil && err != sql.ErrNoRows {
					return nil, err
				}

				// no ticket commands have any effect then
				if activeTicket == nil && !conf.Enabled {
					return createTicketsDisabledError(data.GuildData), nil
				}

				ctx := context.WithValue(data.Context(), CtxKeyConfig, conf)

				if activeTicket != nil {
					participants, _ := models.TicketParticipants(qm.Where("ticket_guild_id = ? AND ticket_local_id = ?", activeTicket.GuildID, activeTicket.LocalID)).AllG(ctx)
					ctx = context.WithValue(ctx, CtxKeyCurrentTicket, &Ticket{
						Ticket:       activeTicket,
						Participants: participants,
					})
				}

				return inner(data.WithContext(ctx))
			}
		})

	container.AddCommand(cmdAddParticipant, cmdAddParticipant.GetTrigger().SetMiddlewares(RequireActiveTicketMW))
	//container.AddCommand(cmdRemoveParticipant, cmdRemoveParticipant.GetTrigger().SetMiddlewares(RequireActiveTicketMW))
	container.AddCommand(cmdCloseTicket, cmdCloseTicket.GetTrigger().SetMiddlewares(RequireActiveTicketMW))
	//container.AddCommand(cmdAdminsOnly, cmdAdminsOnly.GetTrigger().SetMiddlewares(RequireActiveTicketMW))

	commands.RegisterSlashCommandsContainer(container, false, TicketCommandsRolesRunFuncfunc)
}

func TicketCommandsRolesRunFuncfunc(gs *dstate.GuildSet) ([]int64, error) {
	conf, err := models.FindTicketConfigG(context.Background(), gs.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}

		conf = &models.TicketConfig{}
	}

	if !conf.Enabled {
		return nil, nil
	}

	// use the everyone role to signify that everyone can use the commands
	return []int64{gs.ID}, nil
}

func RequireActiveTicketMW(inner dcmd.RunFunc) dcmd.RunFunc {
	return func(data *dcmd.Data) (interface{}, error) {
		if data.Context().Value(CtxKeyCurrentTicket) == nil {
			return "This command can only be ran in a active ticket", nil
		}

		return inner(data)
	}
}

type CtxKey int

const (
	CtxKeyConfig        CtxKey = iota
	CtxKeyCurrentTicket CtxKey = iota
)

type Ticket struct {
	Ticket       *models.Ticket
	Participants []*models.TicketParticipant
}

func createLogs(parsed *dcmd.Data, conf *models.TicketConfig, ticket *models.Ticket, adminOnly bool, embed *discordgo.MessageEmbed) error {

	if !conf.TicketsUseTXTTranscripts && !conf.DownloadAttachments {
		return nil // nothing to do here
	}

	channelID := ticket.ChannelID

	attachments := make([][]*discordgo.MessageAttachment, 0)

	msgs := make([]*discordgo.Message, 0, 100)
	before := int64(0)

	totalAttachmentSize := 0
	for {
		m, err := common.BotSession.ChannelMessages(channelID, 100, before, 0, 0)
		if err != nil {
			return err
		}

		for _, msg := range m {
			// download attachments
		OUTER:
			for _, att := range msg.Attachments {
				msg.Content += fmt.Sprintf("(attachment: %s)", att.Filename)

				totalAttachmentSize += att.Size
				if totalAttachmentSize > 500000000 {
					// above 500MB, ignore...
					break
				}

				// group them up
				for i, ag := range attachments {
					combinedSize := 0
					for _, a := range ag {
						combinedSize += a.Size
					}

					if att.Size+combinedSize > 40000000 {
						continue
					}

					// room left in this zip file
					attachments[i] = append(ag, att)
					continue OUTER
				}

				// we didn't find a grouping
				attachments = append(attachments, []*discordgo.MessageAttachment{att})
			}
		}

		// either continue fetching more or append to messages slice
		if conf.TicketsUseTXTTranscripts {
			msgs = append(msgs, m...)
		}

		if len(msgs) > 100000 {
			break // hard limit at 100k
		}

		if len(m) == 100 {
			// More...
			before = m[len(m)-1].ID
		} else {
			break
		}
	}

	if conf.TicketsUseTXTTranscripts && parsed.GuildData.GS.GetChannel(transcriptChannel(conf, adminOnly)) != nil {
		formattedTranscript, textTranscript := createTXTTranscript(ticket, msgs)

		channel := transcriptChannel(conf, adminOnly)
		_, err := common.BotSession.ChannelMessageSendComplex(channel, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
			Files:  []*discordgo.File{{Name: fmt.Sprintf("transcript-%d-%s.txt", ticket.LocalID, ticket.Title), Reader: formattedTranscript}},
		})
		if err != nil {
			return err
		}

		ticket.Logs = textTranscript
	}

	// compress and send the attachments
	if conf.DownloadAttachments && parsed.GuildData.GS.GetChannel(transcriptChannel(conf, adminOnly)) != nil {
		archiveAttachments(conf, ticket, attachments, adminOnly)
	}
	_, _ = ticket.UpdateG(parsed.Context(), boil.Whitelist("logs"))

	return nil
}

func archiveAttachments(conf *models.TicketConfig, ticket *models.Ticket, groups [][]*discordgo.MessageAttachment, adminOnly bool) {
	var buf bytes.Buffer
	for _, ag := range groups {
		if len(ag) == 1 {
			resp, err := http.Get(ag[0].URL)
			if err != nil {
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				continue
			}

			fName := fmt.Sprintf("attachments-%d-%s-%s", ticket.LocalID, ticket.Title, ag[0].Filename)
			_, _ = common.BotSession.ChannelFileSendWithMessage(transcriptChannel(conf, adminOnly),
				fName, fName, resp.Body)
			continue
		}

		// zip multiple files togheter
		zw := zip.NewWriter(&buf)
		for _, v := range ag {

			resp, err := http.Get(v.URL)
			if err != nil {
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				continue
			}

			f, err := zw.Create(v.Filename)
			if err != nil {
				logger.WithError(err).Info("failed creating zip file")
				continue
			}

			_, err = io.Copy(f, resp.Body)
			if err != nil {
				continue
			}

		}

		zw.Close()
		fname := fmt.Sprintf("attachments-%d-%s.zip", ticket.LocalID, ticket.Title)
		_, err := common.BotSession.ChannelFileSendWithMessage(transcriptChannel(conf, adminOnly), fname, fname, &buf)
		buf.Reset()

		if err != nil {
			logger.WithError(err).WithField("guild", ticket.GuildID).WithField("ticket", ticket.LocalID).Error("[tickets] failed archiving batch of attachments")
		}
	}
}

const TicketTXTDateFormat = "2006 Jan 02 15:04:05"

func createTXTTranscript(ticket *models.Ticket, msgs []*discordgo.Message) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	var text string
	title := fmt.Sprintf("Transcript of ticket #%d - %s, opened by %s at %s, closed at %s.\n\n",
		ticket.LocalID, ticket.Title, ticket.AuthorUsernameDiscrim, ticket.CreatedAt.UTC().Format(TicketTXTDateFormat), ticket.ClosedAt.Time.UTC().Format(TicketTXTDateFormat))

	buf.WriteString(title)
	text += title
	// traverse reverse for correct order (they come in with new-old order, we want old-new)
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]

		// serialize message content
		ts, _ := m.Timestamp.Parse()
		msgContent := fmt.Sprintf("[%s] %s: ", ts.UTC().Format(TicketTXTDateFormat), m.Author.String())
		buf.WriteString(msgContent)
		text += msgContent

		if m.Content != "" {
			buf.WriteString(m.Content)
			text += m.Content

			if len(m.Embeds) > 0 {
				buf.WriteString(", ")
			}
		}

		// serialize embeds
		for _, v := range m.Embeds {
			marshalled, err := json.Marshal(v)
			if err != nil {
				continue
			}

			buf.Write(marshalled)
		}

		text += "\n"
		buf.WriteRune('\n')
	}

	return &buf, text
}

func ticketIsAdminOnly(conf *models.TicketConfig, cs *dstate.ChannelState) bool {

	isAdminsOnlyCurrently := true

	for _, ow := range cs.PermissionOverwrites {
		if ow.Type == discordgo.PermissionOverwriteTypeRole && common.ContainsInt64Slice(conf.ModRoles, ow.ID) {
			if (ow.Allow & InTicketPerms) == InTicketPerms {
				// one of the mod roles has ticket perms, this is not a admin ticket currently
				isAdminsOnlyCurrently = false
			}
		}
	}

	return isAdminsOnlyCurrently
}

func transcriptChannel(conf *models.TicketConfig, adminOnly bool) int64 {
	if adminOnly && conf.TicketsTranscriptsChannelAdminOnly != 0 {
		return conf.TicketsTranscriptsChannelAdminOnly
	}

	return conf.TicketsTranscriptsChannel
}

func createTicketChannel(conf *models.TicketConfig, gs *dstate.GuildSet, authorID int64) (int64, *discordgo.Channel, error) {
	// assemble the permission overwrites for the channel were about to create
	overwrites := []*discordgo.PermissionOverwrite{
		{
			Type:  discordgo.PermissionOverwriteTypeMember,
			ID:    authorID,
			Allow: InTicketPerms,
		},
		{
			Type: discordgo.PermissionOverwriteTypeRole,
			ID:   gs.ID,
			Deny: InTicketPerms,
		},
		{
			Type:  discordgo.PermissionOverwriteTypeMember,
			ID:    common.BotUser.ID,
			Allow: InTicketPerms,
		},
	}

	// add all the mod and admin roles
OUTER:
	for _, v := range conf.ModRoles {
		for _, po := range overwrites {
			if po.Type == discordgo.PermissionOverwriteTypeRole && po.ID == v {
				po.Allow |= InTicketPerms
				continue OUTER
			}
		}

		// not found in existing
		overwrites = append(overwrites, &discordgo.PermissionOverwrite{
			Type:  discordgo.PermissionOverwriteTypeRole,
			ID:    v,
			Allow: InTicketPerms,
		})
	}

	// add admin roles
OUTER2:
	for _, v := range conf.AdminRoles {
		for _, po := range overwrites {
			if po.Type == discordgo.PermissionOverwriteTypeRole && po.ID == v {
				po.Allow |= InTicketPerms
				continue OUTER2
			}
		}

		// not found in existing
		overwrites = append(overwrites, &discordgo.PermissionOverwrite{
			Type:  discordgo.PermissionOverwriteTypeRole,
			ID:    v,
			Allow: InTicketPerms,
		})
	}

	// generate the ID for this ticket
	id, err := common.GenLocalIncrID(gs.ID, "ticket")
	if err != nil {
		return 0, nil, err
	}

	channel, err := common.BotSession.GuildChannelCreateWithOverwrites(gs.ID, fmt.Sprintf("ticket-%04d", id), discordgo.ChannelTypeGuildText, conf.TicketsChannelCategory, overwrites)
	if err != nil {
		return 0, nil, err
	}

	return id, channel, nil
}

func applyChannelParentSettingsOverwrites(parentOverwrites []*discordgo.PermissionOverwrite, newChannelOverwrites []*discordgo.PermissionOverwrite) []*discordgo.PermissionOverwrite {
OUTER:
	for _, v := range parentOverwrites {
		for _, nov := range newChannelOverwrites {
			if nov.Type == v.Type && nov.ID == v.ID {

				nov.Deny |= v.Deny
				nov.Allow |= v.Allow

				// 0 the overlapping bits on the denies
				nov.Deny ^= (nov.Deny & nov.Allow)

				continue OUTER
			}
		}

		// did not find existing overwrite, make a new one
		cop := *v
		newChannelOverwrites = append(newChannelOverwrites, &cop)
	}

	return newChannelOverwrites
}
