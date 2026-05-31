package services

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

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
	// 1. Resolver Destinatarios (BD con fallback al .env)
	var toEmails []string
	if len(recipients) > 0 {
		for _, r := range recipients {
			toEmails = append(toEmails, r.Email)
		}
	} else {
		internalRecipientsRaw := os.Getenv("EMAIL_INTERNAL_RECIPIENTS")
		if internalRecipientsRaw != "" {
			toEmails = strings.Split(internalRecipientsRaw, ",")
		}
	}

	if len(toEmails) == 0 {
		e.logger.Warn("no recipients found to deliver approval notification")
		return nil
	}

	// 2. Limpieza y formateo estricto de variables para el HTML (Estilo CAD)
	companyName := "Unknown"
	if details.CompanyName != nil {
		companyName = *details.CompanyName
	}

	setupCost := "$0.00 CAD"
	if details.InitialSetupCost != nil {
		setupCost = fmt.Sprintf("$%.2f CAD", *details.InitialSetupCost)
	}

	recurringCost := "$0.00 CAD"
	if details.RecurringServiceCost != nil {
		recurringCost = fmt.Sprintf("$%.2f CAD", *details.RecurringServiceCost)
	}

	// Limpiar formato de fecha UTC horrible de Go
	approvedDateStr := time.Now().Format("January 02, 2006")
	if details.ApprovedDate != nil {
		approvedDateStr = details.ApprovedDate.Format("January 02, 2006")
	}

	inspectorNameStr := "—"
	if details.Inspector != nil && details.Inspector.FullName != nil {
		inspectorNameStr = *details.Inspector.FullName
	}

	// Estructura de mapeo idéntica a los tokens {{ . }} del archivo HTML
	emailData := struct {
		PortalName           string
		CompanyName          string
		ContactPersonName    string
		ServiceAddress       string
		BillingAddress       string
		BillingTerms         string
		InitialSetupCost     string
		RecurringServiceCost string
		ServiceFrequency     string
		ProposalDriveLink    string
		ApprovedByName       string
		ApprovedDate         string
		InspectorName        string
		TransitionNotes      string
	}{
		PortalName:           "Phantom Portal",
		CompanyName:          companyName,
		ContactPersonName:    safeStr(details.ContactPersonName),
		ServiceAddress:       safeStr(details.ServiceAddress),
		BillingAddress:       safeStr(details.BillingAddress),
		BillingTerms:         strings.ToUpper(strings.ReplaceAll(safeStr((*string)(details.BillingTerms)), "_", " ")),
		InitialSetupCost:     setupCost,
		RecurringServiceCost: recurringCost,
		ServiceFrequency:     strings.ToUpper(safeFrequency(details.ServiceFrequency, details.FrequencyInterval)),
		ProposalDriveLink:    safeStr(details.ProposalDriveLink),
		ApprovedByName:       safeStr(details.ApprovedByName),
		ApprovedDate:         approvedDateStr,
		InspectorName:        inspectorNameStr,
		TransitionNotes:      safeStr(details.Notes),
	}

	// 3. Cargar y renderizar la plantilla HTML
	tmpl, err := template.ParseFiles("internal/templates/commercial_approved.html")
	if err != nil {
		return fmt.Errorf("failed to open html layout: %w", err)
	}

	var bodyBuffer bytes.Buffer
	if err := tmpl.Execute(&bodyBuffer, emailData); err != nil {
		return fmt.Errorf("failed to map fields to template: %w", err)
	}

	// 4. Enviar usando el método v3 que inyecta HTML nativo
	subject := fmt.Sprintf("🚨 Installation Required: %s", companyName)
	return e.sendWithResend(toEmails, subject, bodyBuffer.String())
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
