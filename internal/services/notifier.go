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

// SendPendingReminder envía un único correo consolidado al inspector con su agenda de mañana.
func (e *EmailNotifier) SendPendingReminder(
	ctx context.Context,
	inspectorName string,
	inspectorEmail string,
	reminders []repositories.PendingReminderRow, // Pasamos la lista completa de sus leads
) error {

	if inspectorEmail == "" || len(reminders) == 0 {
		return nil
	}

	subject := "📅 Action Required: Your Follow-up Schedule for Tomorrow"

	// 1. Construir las filas de la tabla HTML dinámicamente en Go
	var tableRows strings.Builder
	for _, r := range reminders {
		tableRows.WriteString(fmt.Sprintf(`
			<tr style="border-bottom: 1px solid #e5e7eb;">
				<td style="padding: 12px 0; font-size: 14px; font-weight: 600; color: #111827;">%s</td>
				<td style="padding: 12px 0; font-size: 14px; text-align: right;">
					<a href="https://crm.phantompestcontrol.ca/dashboard/clients/%s" target="_blank" style="color: #d97706; text-decoration: none; font-weight: 600;">Open File →</a>
				</td>
			</tr>
		`, r.CompanyName, r.ClientID.String()))
	}

	// 2. Estructura HTML profesional con los estilos de Phantom Portal
	htmlBody := fmt.Sprintf(`
	<!DOCTYPE html>
	<html>
	<head><meta charset="UTF-8"></head>
	<body style="margin: 0; padding: 0; font-family: 'Segoe UI', sans-serif; background-color: #f4f4f6; color: #1f2937;">
		<table align="center" border="0" cellpadding="0" cellspacing="0" width="100%%" style="max-width: 600px; margin: 20px auto; background-color: #ffffff; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0,0,0,0.1);">
			<tr>
				<td style="background-color: #111827; padding: 30px; text-align: center; border-bottom: 4px solid #d97706;">
					<h1 style="color: #ffffff; margin: 0; font-size: 20px; font-weight: 600;">Phantom Portal</h1>
					<p style="color: #9ca3af; margin: 5px 0 0 0; font-size: 12px; text-transform: uppercase;">Daily Agenda Briefing</p>
				</td>
			</tr>
			<tr>
				<td style="padding: 40px 30px;">
					<p style="margin: 0 0 20px 0; font-size: 15px; line-height: 1.5;">
						Hi <strong>%s</strong>,<br><br>
						This is your daily briefing. Tomorrow you have <strong>%d pending commercial lead(s)</strong> that require immediate follow-up and contact.
					</p>
					
					<h2 style="font-size: 12px; font-weight: 700; text-transform: uppercase; color: #d97706; margin: 30px 0 10px 0; letter-spacing: 0.05em;">Leads to Contact Tomorrow</h2>
					<table width="100%%" style="border-collapse: collapse; margin-bottom: 20px;">
						%s
					</table>
					
					<p style="font-size: 13px; color: #6b7280; margin-top: 30px;">
						Please ensure to log all notes, outcome details, and status adjustments directly in the portal after communicating with each client.
					</p>
				</td>
			</tr>
			<tr>
				<td style="background-color: #f3f4f6; padding: 20px 30px; text-align: center; font-size: 11px; color: #9ca3af; border-top: 1px solid #e5e7eb;">
					Automated Sales Engine • Phantom Pest Control 2026
				</td>
			</tr>
		</table>
	</body>
	</html>
	`, inspectorName, len(reminders), tableRows.String())

	// 3. Despachar el correo vía tu método v3 de Resend
	return e.sendWithResend([]string{inspectorEmail}, subject, htmlBody)
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

func (l *LogNotifier) SendPendingReminder(
	_ context.Context,
	inspectorName string,
	inspectorEmail string,
	reminders []repositories.PendingReminderRow, // ◄ Actualizado
) error {
	l.logger.Info("NOTIFY: pending reminders consolidated briefing",
		zap.String("inspector_name", inspectorName),
		zap.String("inspector_email", inspectorEmail),
		zap.Int("total_leads_assigned", len(reminders)),
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
