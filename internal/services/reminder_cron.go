package services

import (
	"context"
	"time"

	"github.com/phantompestcontrol/crm/internal/repositories"
	"go.uber.org/zap"
)

// ReminderCron runs as a background goroutine.
// It fires once per day and sends follow-up reminders for pending
// commercial clients whose next_followup_date is tomorrow.
//
// Design choices:
//   - Pure Go ticker — no DB cron, no Redis, no external scheduler
//   - Fires once daily at a configurable UTC hour (default 09:00 UTC)
//   - Non-blocking: misses during downtime are acceptable (1 day cadence)
//   - Idempotent: re-running on the same day sends duplicate reminders,
//     so we track last run date and skip if already ran today
type ReminderCron struct {
	commercialRepo *repositories.CommercialRepository
	notifier       NotificationSender
	logger         *zap.Logger
	runHourUTC     int    // hour of day to fire (0–23), default 9
	lastRunDate    string // YYYY-MM-DD, prevents double-firing
}

func NewReminderCron(
	commercialRepo *repositories.CommercialRepository,
	notifier NotificationSender,
	logger *zap.Logger,
	runHourUTC int,
) *ReminderCron {
	if runHourUTC < 0 || runHourUTC > 23 {
		runHourUTC = 9
	}
	return &ReminderCron{
		commercialRepo: commercialRepo,
		notifier:       notifier,
		logger:         logger,
		runHourUTC:     runHourUTC,
	}
}

// Start launches the cron loop. Call via go cron.Start(ctx).
// Graceful shutdown: cancel the context to stop.
func (c *ReminderCron) Start(ctx context.Context) {
	c.logger.Info("reminder cron started", zap.Int("run_hour_utc", c.runHourUTC))

	ticker := time.NewTicker(1 * time.Minute) // check every minute
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("reminder cron stopped")
			return
		case t := <-ticker.C:
			utc := t.UTC()
			if utc.Hour() == c.runHourUTC && utc.Minute() == 0 {
				today := utc.Format(time.DateOnly)
				if c.lastRunDate == today {
					continue // already ran today
				}
				c.lastRunDate = today
				c.runReminders(ctx, utc)
			}
		}
	}
}

// runReminders sends notifications for clients with next_followup_date = tomorrow.
func (c *ReminderCron) runReminders(ctx context.Context, now time.Time) {
	tomorrow := now.Add(24 * time.Hour).Truncate(24 * time.Hour)

	c.logger.Info("running pending follow-up reminders",
		zap.String("target_date", tomorrow.Format(time.DateOnly)),
	)

	rows, err := c.commercialRepo.GetPendingFollowupsDue(ctx, tomorrow)
	if err != nil {
		c.logger.Error("failed to fetch pending follow-ups", zap.Error(err))
		return
	}

	if len(rows) == 0 {
		c.logger.Info("no follow-up reminders to send today")
		return
	}

	sent := 0
	for _, row := range rows {
		if err := c.notifier.SendPendingReminder(ctx, row); err != nil {
			c.logger.Error("failed to send pending reminder",
				zap.String("client_id", row.ClientID.String()),
				zap.Error(err),
			)
			continue
		}
		sent++
	}

	c.logger.Info("pending reminders sent",
		zap.Int("total", len(rows)),
		zap.Int("sent", sent),
	)
}

// RunNow triggers an immediate run (useful for testing or manual trigger via admin API).
func (c *ReminderCron) RunNow(ctx context.Context) {
	c.runReminders(ctx, time.Now().UTC())
}
