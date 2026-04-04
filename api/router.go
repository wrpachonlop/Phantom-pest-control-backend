package api

import (
	"net/http"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phantompestcontrol/crm/internal/config"
	"github.com/phantompestcontrol/crm/internal/handlers"
	"github.com/phantompestcontrol/crm/internal/middleware"
	"github.com/phantompestcontrol/crm/internal/repositories"
	"github.com/phantompestcontrol/crm/internal/services"
	"go.uber.org/zap"
	"strings"
)

// NewRouter constructs the Gin engine with all routes and middleware wired.
func NewRouter(cfg *config.Config, db *pgxpool.Pool, logger *zap.Logger) *gin.Engine {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger(logger))

	// CORS
	allowedOrigins := strings.Split(cfg.AllowedOrigins, ",")
	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// ── Dependencies ──────────────────────────────────────────

	auditSvc := middleware.NewAuditService(db, logger)

	// Repositories
	clientRepo := repositories.NewClientRepository(db)
	reportRepo := repositories.NewReportRepository(db)

	// Services
	clientSvc := services.NewClientService(clientRepo, auditSvc, logger)
	reportSvc := services.NewReportService(reportRepo, logger)
	followUpSvc := services.NewFollowUpService(db, auditSvc, logger)

	// Handlers
	clientH := handlers.NewClientHandler(clientSvc, logger)
	reportH := handlers.NewReportHandler(reportSvc, logger)
	followUpH := handlers.NewFollowUpHandler(followUpSvc, logger)
	adminH := handlers.NewAdminHandler(db, auditSvc, logger)

	// ── Public Routes ─────────────────────────────────────────

	r.GET("/health", adminH.HealthCheck)

	// ── Auth-Protected Routes ─────────────────────────────────

	auth := r.Group("/api/v1")
	auth.Use(middleware.RequireAuth(cfg))
	{
		// Me
		auth.GET("/me", adminH.GetMe)
		auth.POST("/me/sync", adminH.UpsertMe)

		// Clients
		auth.GET("/clients", clientH.List)
		auth.GET("/clients/:id", clientH.Get)
		auth.POST("/clients", clientH.Create)
		auth.PUT("/clients/:id", clientH.Update)
		auth.GET("/clients/:id/follow-ups", followUpH.GetByClient)
		auth.GET("/clients/:id/notes", adminH.GetNotesByClient)
		auth.GET("/clients/:id/audit", clientH.GetAuditLog)

		// Duplicate detection
		auth.POST("/clients/check-duplicates", clientH.CheckDuplicates)

		// Follow-ups
		auth.POST("/follow-ups", followUpH.Create)

		// Notes
		auth.POST("/notes", adminH.CreateNote)

		// Lookup tables (read access for all authenticated users)
		auth.GET("/contact-methods", adminH.ListContactMethods)
		auth.GET("/pest-issues", adminH.ListPestIssues)

		// Reports
		auth.GET("/reports", reportH.GetReport)
		auth.GET("/reports/dashboard", reportH.GetDashboard)

		// ── Admin-only routes ─────────────────────────────────
		admin := auth.Group("")
		admin.Use(middleware.RequireAdmin())
		{
			// Client delete
			admin.DELETE("/clients/:id", clientH.Delete)

			// Follow-up delete
			admin.DELETE("/follow-ups/:id", followUpH.Delete)

			// Note delete
			admin.DELETE("/notes/:id", adminH.DeleteNote)

			// Contact methods management
			admin.POST("/contact-methods", adminH.CreateContactMethod)
			admin.PUT("/contact-methods/:id", adminH.UpdateContactMethod)

			// Pest issues management
			admin.POST("/pest-issues", adminH.CreatePestIssue)
			admin.PUT("/pest-issues/:id", adminH.UpdatePestIssue)

			// User management
			admin.GET("/admin/users", adminH.ListUsers)
			admin.PUT("/admin/users/:id/role", adminH.UpdateUserRole)

			// Audit log (global)
			admin.GET("/audit-logs", adminH.GetAuditLog)
		}
	}

	// 404 handler
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
	})

	return r
}
