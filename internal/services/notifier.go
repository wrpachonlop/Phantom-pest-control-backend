package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/phantompestcontrol/crm/internal/models"
	"github.com/phantompestcontrol/crm/internal/repositories"
	"github.com/resend/resend-go/v3"
	"go.uber.org/zap"
)

// EmailNotifier implements NotificationSender using SMTP.
// In production, swap for a SendGrid/Mailgun client.
type EmailNotifier struct {
	resendClient *resend.Client // ◄ Reemplaza smtpHost, smtpPort, smtpUser, etc.
	fromAddr     string
	logger       *zap.Logger
}

func NewEmailNotifier(logger *zap.Logger) *EmailNotifier {
	apiKey := os.Getenv("RESEND_API_KEY")
	client := resend.NewClient(apiKey)

	return &EmailNotifier{
		resendClient: client,
		// Por ahora en tu .env EMAIL_FROM será onboarding@resend.dev
		fromAddr: getEnvOrDefault("EMAIL_FROM", "onboarding@resend.dev"),
		logger:   logger,
	}
}

// SendCommercialApproved sends approval notifications to all configured recipients.
func (e *EmailNotifier) SendCommercialApproved(
	ctx context.Context,
	details *models.CommercialClientDetails,
	recipients []models.NotificationRecipient,
) error {
	fmt.Printf("Preparing to send commercial approved notification for client %s to %d recipients\n", details.ClientID, len(recipients))
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
		fmt.Printf("Sending commercial approved notification to %s\n", r.Email)
		if err := e.sendWithResend([]string{r.Email}, subject, body); err != nil {
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

	return e.sendWithResend([]string{row.InspectorEmail}, subject, body)
}

func (e *EmailNotifier) sendWithResend(to []string, subject, htmlBody string) error {
	// Si no configuras la API Key en desarrollo, lo saca por consola para que no falle el backend
	if os.Getenv("RESEND_API_KEY") == "" {
		fmt.Printf("[RESEND MOCK] To: %v\nSubject: %s\nHTML:\n%s\n", to, subject, htmlBody)
		return nil
	}

	params := &resend.SendEmailRequest{
		From:    e.fromAddr,
		To:      to,
		Subject: subject,
		Html:    htmlBody,
	}

	// ── CAMBIO CRÍTICO V3: Usamos Send(params) directo como dicta la documentación ──
	sent, err := e.resendClient.Emails.Send(params)
	if err != nil {
		e.logger.Error("failed to deliver email via Resend API v3", zap.Strings("recipients", to), zap.Error(err))
		return err
	}

	e.logger.Info("Email payload processed by Phantom Portal successfully",
		zap.Strings("recipients", to),
		zap.String("email_id", sent.Id),
	)
	return nil
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
