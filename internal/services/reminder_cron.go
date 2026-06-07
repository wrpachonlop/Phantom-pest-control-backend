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
	c.logger.Info("reminder cron engine initialized for 10:00 AM (Mon-Fri)")

	go func() {
		for {
			now := time.Now()

			// 1. Calcular cuándo deberían ser las próximas 10:00 AM en la hora del servidor
			nextRun := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())

			// Si ya pasaron las 10:00 AM de hoy, calculamos las 10:00 AM de mañana
			if now.After(nextRun) {
				nextRun = nextRun.Add(24 * time.Hour)
			}

			// 2. Calcular la duración exacta que el proceso debe dormir
			durationUntilNextRun := time.Until(nextRun)
			c.logger.Info("Scheduling next follow-up reminders check",
				zap.String("next_run_target", nextRun.Format("2006-01-02 15:04:05")),
				zap.Duration("wait_time", durationUntilNextRun),
			)

			select {
			case <-time.After(durationUntilNextRun):
				// 3. Verificar si el día objetivo es fin de semana (Sábado o Domingo)
				// Con esto nos aseguramos de que SOLO se ejecute de Lunes a Viernes
				weekday := time.Now().Weekday()
				if weekday == time.Saturday || weekday == time.Sunday {
					c.logger.Info("Skipping reminders execution: Weekend detected",
						zap.String("day", weekday.String()),
					)
					continue
				}

				// 4. ¡Disparar los recordatorios consolidados!
				c.logger.Info("Executing scheduled daily reminder cron job...")
				c.runReminders(ctx, time.Now())

			case <-ctx.Done():
				c.logger.Info("reminder cron stopped gracefully")
				return
			}
		}
	}()
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
	type InspectorGroup struct {
		Name string
		Rows []repositories.PendingReminderRow
	}
	groupedReminders := make(map[string]*InspectorGroup)

	for _, row := range rows {
		email := row.InspectorEmail
		if email == "" {
			continue
		}

		// Determinar un nombre por si viene nulo
		name := "Inspector"
		if row.InspectorName != nil {
			name = *row.InspectorName
		}

		// Si el inspector no está en el mapa, lo inicializamos
		if _, exists := groupedReminders[email]; !exists {
			groupedReminders[email] = &InspectorGroup{
				Name: name,
				Rows: []repositories.PendingReminderRow{},
			}
		}

		// Agregamos la fila a su lista
		groupedReminders[email].Rows = append(groupedReminders[email].Rows, row)
	}

	sent := 0
	for email, group := range groupedReminders {
		if err := c.notifier.SendPendingReminder(ctx, group.Name, email, group.Rows); err != nil {
			c.logger.Error("failed to send consolidated pending reminder",
				zap.String("inspector_email", email),
				zap.Error(err),
			)
			continue
		}
		sent++
	}

	c.logger.Info("pending reminders execution completed",
		zap.Int("total_inspectors_notified", sent),
	)
}

// RunNow triggers an immediate run (useful for testing or manual trigger via admin API).
func (c *ReminderCron) RunNow(ctx context.Context) {
	c.runReminders(ctx, time.Now().UTC())
}
