package services

import (
	"context"
	"fmt"
	"net/smtp"
	"os"
	"strings"

	"github.com/phantompestcontrol/crm/internal/models"
	"github.com/phantompestcontrol/crm/internal/repositories"
	"go.uber.org/zap"
)

// EmailNotifier implements NotificationSender using SMTP.
// In production, swap for a SendGrid/Mailgun client.
type EmailNotifier struct {
	smtpHost string
	smtpPort string
	smtpUser string
	smtpPass string
	fromAddr string
	logger   *zap.Logger
}

func NewEmailNotifier(logger *zap.Logger) *EmailNotifier {
	return &EmailNotifier{
		smtpHost: getEnvOrDefault("SMTP_HOST", "smtp.gmail.com"),
		smtpPort: getEnvOrDefault("SMTP_PORT", "587"),
		smtpUser: os.Getenv("SMTP_USER"),
		smtpPass: os.Getenv("SMTP_PASS"),
		fromAddr: getEnvOrDefault("SMTP_FROM", "crm@phantompestcontrol.ca"),
		logger:   logger,
	}
}

// SendCommercialApproved sends approval notifications to all configured recipients.
func (e *EmailNotifier) SendCommercialApproved(
	ctx context.Context,
	details *models.CommercialClientDetails,
	recipients []models.NotificationRecipient,
) error {
	if len(recipients) == 0 {
		e.logger.Warn("no recipients configured for commercial_approved event")
		return nil
	}

	companyName := "Unknown"
	if details.CompanyName != nil {
		companyName = *details.CompanyName
	}

	subject := fmt.Sprintf("[Phantom CRM] Commercial Client Approved: %s", companyName)

	body := fmt.Sprintf(`
A commercial client has been approved in Phantom Pest Control CRM.

Company: %s
Approved By: %s
Approved Date: %s
Initial Setup Cost: $%.2f
Recurring Service Cost: $%.2f
Service Frequency: %s

View in CRM: https://crm.phantompestcontrol.ca/dashboard/clients/%s

This is an automated notification from Phantom Pest Control CRM.
`,
		companyName,
		safeStr(details.ApprovedByName),
		safeDate(details.ApprovedDate),
		safeFloat(details.InitialSetupCost),
		safeFloat(details.RecurringServiceCost),
		safeFrequency(details.ServiceFrequency, details.FrequencyInterval),
		details.ClientID.String(),
	)

	for _, r := range recipients {
		if err := e.send(r.Email, subject, body); err != nil {
			e.logger.Error("failed to send approval email",
				zap.String("recipient", r.Email),
				zap.Error(err),
			)
			// Continue sending to other recipients even if one fails
		}
	}
	return nil
}

// SendPendingReminder sends a 1-day-before follow-up reminder to the inspector.
func (e *EmailNotifier) SendPendingReminder(
	ctx context.Context,
	row repositories.PendingReminderRow,
) error {
	if row.InspectorEmail == "" {
		return nil
	}

	inspectorName := "Inspector"
	if row.InspectorName != nil {
		inspectorName = *row.InspectorName
	}

	subject := fmt.Sprintf("[Phantom CRM] Follow-up reminder: %s tomorrow", row.CompanyName)
	body := fmt.Sprintf(`
Hi %s,

This is a reminder that you have a scheduled follow-up for a commercial client tomorrow.

Company: %s
Follow-up Date: %s

Please log your follow-up in the CRM:
https://crm.phantompestcontrol.ca/dashboard/clients/%s

Phantom Pest Control CRM
`,
		inspectorName,
		row.CompanyName,
		row.NextFollowupDate.Format("January 2, 2006"),
		row.ClientID.String(),
	)

	return e.send(row.InspectorEmail, subject, body)
}

func (e *EmailNotifier) send(to, subject, body string) error {
	if e.smtpUser == "" || e.smtpPass == "" {
		// Dev mode: log instead of send
		fmt.Printf("[EMAIL NOTIFIER] To: %s\nSubject: %s\n%s\n", to, subject, body)
		return nil
	}

	auth := smtp.PlainAuth("", e.smtpUser, e.smtpPass, e.smtpHost)
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		e.fromAddr, to, subject, body,
	)

	return smtp.SendMail(
		e.smtpHost+":"+e.smtpPort,
		auth,
		e.fromAddr,
		[]string{to},
		[]byte(msg),
	)
}

// ─── Log-only notifier for testing / staging ─────────────────────────────────

type LogNotifier struct {
	logger *zap.Logger
}

func NewLogNotifier(logger *zap.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

func (l *LogNotifier) SendCommercialApproved(_ context.Context, details *models.CommercialClientDetails, recipients []models.NotificationRecipient) error {
	l.logger.Info("NOTIFY: commercial approved",
		zap.String("client_id", details.ClientID.String()),
		zap.Int("recipient_count", len(recipients)),
	)
	return nil
}

func (l *LogNotifier) SendPendingReminder(_ context.Context, row repositories.PendingReminderRow) error {
	l.logger.Info("NOTIFY: pending reminder",
		zap.String("client_id", row.ClientID.String()),
		zap.String("company", row.CompanyName),
		zap.String("inspector_email", row.InspectorEmail),
	)
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func safeStr(s *string) string {
	if s == nil {
		return "—"
	}
	return *s
}

func safeDate(t interface{}) string {
	// handles *time.Time
	return fmt.Sprintf("%v", t)
}

func safeFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func safeFrequency(f *models.ServiceFrequency, interval *int) string {
	if f == nil {
		return "—"
	}
	if interval != nil && models.FrequencySupportsInterval(*f) {
		return fmt.Sprintf("Every %d %s", *interval, strings.TrimSuffix(string(*f), "ly"))
	}
	return string(*f)
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
