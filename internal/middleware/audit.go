package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// AuditService writes audit log entries to the database.
type AuditService struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewAuditService(db *pgxpool.Pool, logger *zap.Logger) *AuditService {
	return &AuditService{db: db, logger: logger}
}

// Log writes an audit entry. Non-blocking – runs in goroutine.
func (a *AuditService) Log(
	userID *uuid.UUID,
	action, entityType string,
	entityID uuid.UUID,
	oldValues, newValues interface{},
	ipAddress, userAgent *string,
) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		oldJSON, _ := json.Marshal(oldValues)
		newJSON, _ := json.Marshal(newValues)

		_, err := a.db.Exec(ctx, `
			INSERT INTO audit_logs
				(user_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8)
		`, userID, action, entityType, entityID, oldJSON, newJSON, ipAddress, userAgent)

		if err != nil {
			a.logger.Error("failed to write audit log",
				zap.Error(err),
				zap.String("entity_type", entityType),
				zap.String("entity_id", entityID.String()),
			)
		}
	}()
}

// RequestLogger returns a gin middleware that logs HTTP requests via zap.
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", query),
			zap.String("ip", c.ClientIP()),
			zap.Duration("latency", latency),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		if status >= http.StatusInternalServerError {
			logger.Error("server error", fields...)
		} else if status >= http.StatusBadRequest {
			logger.Warn("client error", fields...)
		} else {
			logger.Info("request", fields...)
		}
	}
}
