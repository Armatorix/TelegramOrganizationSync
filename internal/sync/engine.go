// Package sync owns the reconciliation loop. It pulls channels from the
// org server, snapshots Telegram membership, asks the server for a diff,
// and either applies it (auto) or notifies the manager (manual).
package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Armatorix/TelegramOrganizationSync/internal/api"
	"github.com/Armatorix/TelegramOrganizationSync/internal/config"
	"github.com/Armatorix/TelegramOrganizationSync/internal/server"
	"github.com/Armatorix/TelegramOrganizationSync/internal/telegram"
)

// dangerousRemovalRatio aborts auto-mode application if the server asks
// us to remove more than this fraction of the current member list. The
// engine then falls back to manager notification with a "danger" prefix.
// Defends against a buggy/compromised server emptying a channel.
const dangerousRemovalRatio = 0.5

type Engine struct {
	cfg    config.Config
	server *server.Client
	tg     telegram.Adapter
	log    *slog.Logger
}

func New(cfg config.Config, srv *server.Client, tg telegram.Adapter, log *slog.Logger) *Engine {
	return &Engine{cfg: cfg, server: srv, tg: tg, log: log}
}

func (e *Engine) Run(ctx context.Context) error {
	e.log.Info("sync engine started",
		"mode", e.cfg.Mode,
		"interval", e.cfg.Sync.Interval,
		"dry_run", e.cfg.Sync.DryRun)

	go e.discoveryLoop(ctx)

	// Run an initial tick immediately so dev-loop feedback is fast.
	e.tick(ctx)

	t := time.NewTicker(e.cfg.Sync.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			e.tick(ctx)
		}
	}
}

func (e *Engine) tick(ctx context.Context) {
	tickID := time.Now().UTC().Format("20060102T150405.000")
	log := e.log.With("tick_id", tickID)

	channels, err := e.server.ListChannels(ctx)
	if err != nil {
		log.Warn("list channels failed; skipping tick", "err", err)
		return
	}
	log.Info("tick start", "channels", len(channels))

	for _, ch := range channels {
		if ch.SyncStatus != api.SyncStatusEnabled {
			continue
		}
		e.reconcileChannel(ctx, log, ch)
	}
}

func (e *Engine) reconcileChannel(ctx context.Context, log *slog.Logger, ch api.Channel) {
	log = log.With("channel_id", ch.ID, "telegram_chat_id", ch.TelegramChatID)

	members, err := e.tg.ListMembers(ctx, ch.TelegramChatID)
	if err != nil {
		log.Warn("list telegram members failed", "err", err)
		return
	}

	diff, err := e.server.Reconcile(ctx, ch.ID, api.ReconcileRequest{Members: members})
	if err != nil {
		log.Warn("reconcile call failed", "err", err)
		return
	}

	if len(diff.ToAdd) == 0 && len(diff.ToRemove) == 0 {
		log.Info("channel in sync")
		return
	}

	log.Info("diff received", "add", len(diff.ToAdd), "remove", len(diff.ToRemove))

	if e.cfg.Sync.DryRun {
		log.Info("dry run; not applying", "add", names(diff.ToAdd), "remove", names(diff.ToRemove))
		return
	}

	if e.cfg.Mode == config.ModeManual {
		e.notifyManager(ctx, log, ch, diff, "")
		return
	}

	// Auto mode — safety rail before mutating.
	if dangerousRemoval(len(members), len(diff.ToRemove)) {
		log.Warn("removal exceeds safety threshold; falling back to manager notification",
			"members", len(members), "to_remove", len(diff.ToRemove))
		e.notifyManager(ctx, log, ch, diff,
			"⚠️ refused to auto-apply: removal exceeded safety threshold.\n")
		return
	}

	e.applyAuto(ctx, log, ch, diff)
}

func (e *Engine) applyAuto(ctx context.Context, log *slog.Logger, ch api.Channel, diff api.ReconcileResponse) {
	for _, m := range diff.ToAdd {
		if err := e.tg.AddMember(ctx, ch.TelegramChatID, m.TelegramUserID); err != nil {
			log.Warn("add member failed", "user_id", m.TelegramUserID, "err", err)
			continue
		}
		log.Info("added member", "user_id", m.TelegramUserID, "name", m.Name)
	}
	for _, m := range diff.ToRemove {
		if err := e.tg.RemoveMember(ctx, ch.TelegramChatID, m.TelegramUserID); err != nil {
			log.Warn("remove member failed", "user_id", m.TelegramUserID, "err", err)
			continue
		}
		log.Info("removed member", "user_id", m.TelegramUserID, "name", m.Name)
	}
}

func (e *Engine) notifyManager(ctx context.Context, log *slog.Logger, ch api.Channel, diff api.ReconcileResponse, prefix string) {
	if ch.Manager == nil {
		log.Warn("no manager set; cannot notify")
		return
	}
	body := renderManagerMessage(ch, diff, prefix)
	if err := e.tg.SendDM(ctx, ch.Manager.TelegramUserID, body); err != nil {
		log.Warn("send manager dm failed", "manager_user_id", ch.Manager.TelegramUserID, "err", err)
		return
	}
	log.Info("manager notified", "manager_user_id", ch.Manager.TelegramUserID)
}

func (e *Engine) discoveryLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-e.tg.DiscoveredChannels():
			if !ok {
				return
			}
			_, err := e.server.CreateChannel(ctx, api.CreateChannelRequest{
				TelegramChatID: d.TelegramChatID,
				Title:          d.Title,
			})
			if err != nil {
				e.log.Warn("register discovered channel failed",
					"telegram_chat_id", d.TelegramChatID, "err", err)
				continue
			}
			e.log.Info("discovered channel registered",
				"telegram_chat_id", d.TelegramChatID, "title", d.Title)
		}
	}
}

func dangerousRemoval(currentCount, removeCount int) bool {
	if currentCount == 0 {
		return false
	}
	return float64(removeCount)/float64(currentCount) > dangerousRemovalRatio
}

func renderManagerMessage(ch api.Channel, diff api.ReconcileResponse, prefix string) string {
	var b strings.Builder
	if prefix != "" {
		b.WriteString(prefix)
	}
	fmt.Fprintf(&b, "Pending membership changes for %s\n\n", ch.Title)
	if len(diff.ToAdd) > 0 {
		b.WriteString("To add:\n")
		for _, m := range diff.ToAdd {
			fmt.Fprintf(&b, "  + %s (id %d)\n", m.Name, m.TelegramUserID)
		}
	}
	if len(diff.ToRemove) > 0 {
		b.WriteString("To remove:\n")
		for _, m := range diff.ToRemove {
			fmt.Fprintf(&b, "  - %s (id %d)\n", m.Name, m.TelegramUserID)
		}
	}
	return b.String()
}

func names(ms []api.Member) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.Name
	}
	return out
}
