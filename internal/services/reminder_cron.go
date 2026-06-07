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
	c.logger.Info("Reminder cron initialized in production mode (Checks via internal ticker/monitor)")

	// Dejamos un ticker de 15 minutos (alineado con tu monitor de Render)
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Cargar la zona horaria de Vancouver/Burnaby para que no dependa de la hora UTC de Render
	location, err := time.LoadLocation("America/Vancouver")
	if err != nil {
		c.logger.Warn("Failed to load America/Vancouver timezone, falling back to local system time", zap.Error(err))
		location = time.Local
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Reminder cron stopped gracefully")
			return
		case <-ticker.C:
			// 1. Obtener la hora actual explícitamente en horario de Vancouver
			nowLocal := time.Now().In(location)

			// 2. Verificar si es fin de semana. Si es Sábado o Domingo, ignorar.
			if nowLocal.Weekday() == time.Saturday || nowLocal.Weekday() == time.Sunday {
				continue
			}

			// 3. Verificar si estamos en la ventana de las 10:00 AM
			if nowLocal.Hour() == 10 {
				today := nowLocal.Format(time.DateOnly)

				// Protección estricta: Si ya corrió hoy, no vuelvas a enviar nada en este bloque de 15 minutos
				if c.lastRunDate == today {
					continue
				}

				c.lastRunDate = today
				c.logger.Info("10:00 AM window detected! Triggering daily pending follow-up reminders...")
				c.RunReminders(ctx, nowLocal)
			}
		}
	}
}

// RunReminders sends notifications for clients with next_followup_date = tomorrow.
func (c *ReminderCron) RunReminders(ctx context.Context, now time.Time) {
	tomorrow := now.AddDate(0, 0, 1)

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
	c.RunReminders(ctx, time.Now().UTC())
}
