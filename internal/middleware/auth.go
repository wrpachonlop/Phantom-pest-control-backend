package middleware

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/phantompestcontrol/crm/internal/config"
	"github.com/phantompestcontrol/crm/internal/models"
)

const (
	ContextKeyUserID = "user_id"
	ContextKeyEmail  = "user_email"
	ContextKeyRole   = "user_role"
)

// SupabaseClaims represents JWT claims issued by Supabase Auth.
type SupabaseClaims struct {
	jwt.RegisteredClaims
	Email string `json:"email"`
	Role  string `json:"role"` // Supabase role (authenticated, anon)
	// App metadata set by Supabase user metadata
	UserMetadata map[string]interface{} `json:"user_metadata"`
	AppMetadata  map[string]interface{} `json:"app_metadata"`
}

func parsePublicKey(rawKey string) (interface{}, error) {
	fmt.Print("Starting to parse public key...\n")
	// 1. Quitar comillas accidentales que a veces se cuelan en Render
	cleanKey := strings.Trim(rawKey, "\"")

	// 2. Convertir los "\n" de texto a saltos de línea reales
	cleanKey = strings.ReplaceAll(cleanKey, "\\n", "\n")

	// 3. Limpiar espacios en blanco al inicio y al final
	cleanKey = strings.TrimSpace(cleanKey)

	// 4. Intentar parsear
	block, _ := pem.Decode([]byte(cleanKey))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block: verify BEGIN/END tags are present")
	}

	return jwt.ParseECPublicKeyFromPEM([]byte(cleanKey))
}

// RequireAuth validates the Supabase JWT and restricts to @phantompestcontrol.ca
func RequireAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract Bearer token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}
		tokenStr := parts[1]

		// Parse and validate JWT (Supabase signs with HS256 using JWT secret)
		claims := &SupabaseClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {

			if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}

			// 2. Parsear la llave pública PEM que guardamos en el config
			// Reemplaza los "\n" literales por saltos de línea reales si vienen del .env

			pubKey, err := parsePublicKey(cfg.JWTPublicKey)
			if err != nil {
				return nil, fmt.Errorf("failed to parse EC public key: %w", err)
			}

			return pubKey, nil
			// return []byte(cfg.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			fmt.Printf("DEBUG AUTH ERROR: %v\n", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		// Domain restriction: only @phantompestcontrol.ca
		if !strings.HasSuffix(claims.Email, "@"+cfg.AllowedEmailDomain) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": fmt.Sprintf("access restricted to @%s accounts", cfg.AllowedEmailDomain),
			})
			return
		}

		// Parse user ID from subject claim
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid user ID in token"})
			return
		}

		// Store in context for downstream handlers
		c.Set(ContextKeyUserID, userID)
		c.Set(ContextKeyEmail, claims.Email)
		// Role is read from app_metadata (set by admin via Supabase)
		if role, ok := claims.AppMetadata["role"].(string); ok {
			c.Set(ContextKeyRole, models.UserRole(role))
		} else {
			c.Set(ContextKeyRole, models.UserRoleUser) // default
		}

		c.Next()
	}
}

// RequireAdmin blocks non-admin users.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get(ContextKeyRole)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		if role.(models.UserRole) != models.UserRoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			return
		}

		c.Next()
	}
}

// GetUserID extracts the authenticated user's UUID from gin context.
func GetUserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ContextKeyUserID)
	return v.(uuid.UUID)
}

// GetUserRole extracts the authenticated user's role from gin context.
func GetUserRole(c *gin.Context) models.UserRole {
	v, _ := c.Get(ContextKeyRole)
	return v.(models.UserRole)
}
