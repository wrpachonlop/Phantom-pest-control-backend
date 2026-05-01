package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type LocationHandler struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

func NewLocationHandler(db *pgxpool.Pool, logger *zap.Logger) *LocationHandler {
	return &LocationHandler{
		db:     db,
		logger: logger,
	}
}

// GET /clients/:id/locations
func (h *LocationHandler) ListByClient(c *gin.Context) {
	clientID := c.Param("id")

	rows, err := h.db.Query(c.Request.Context(),
		"SELECT id, label, location_type, location_value, is_primary FROM client_locations WHERE client_id = $1 ORDER BY is_primary DESC, created_at DESC",
		clientID)
	if err != nil {
		h.logger.Error("Error listing locations", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch locations"})
		return
	}
	defer rows.Close()

	var locations []any
	for rows.Next() {
		var l struct {
			ID            string `json:"id"`
			Label         string `json:"label"`
			LocationType  string `json:"location_type"`
			LocationValue string `json:"location_value"`
			IsPrimary     bool   `json:"is_primary"`
		}
		if err := rows.Scan(&l.ID, &l.Label, &l.LocationType, &l.LocationValue, &l.IsPrimary); err != nil {
			continue
		}
		locations = append(locations, l)
	}

	c.JSON(http.StatusOK, locations)
}

// POST /clients/:id/locations
func (h *LocationHandler) Create(c *gin.Context) {
	clientID := c.Param("id")
	var req struct {
		Label         string `json:"label"`
		LocationType  string `json:"location_type"` // 'address' o 'coordinates'
		LocationValue string `json:"location_value"`
		IsPrimary     bool   `json:"is_primary"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	_, err := h.db.Exec(c.Request.Context(),
		"INSERT INTO client_locations (client_id, label, location_type, location_value, is_primary) VALUES ($1, $2, $3, $4, $5)",
		clientID, req.Label, req.LocationType, req.LocationValue, req.IsPrimary)

	if err != nil {
		h.logger.Error("Error creating location", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create location"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Location created successfully"})
}

// PUT /locations/:id
func (h *LocationHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Label         string `json:"label"`
		LocationValue string `json:"location_value"`
		IsPrimary     bool   `json:"is_primary"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input"})
		return
	}

	_, err := h.db.Exec(c.Request.Context(),
		"UPDATE client_locations SET label = $1, location_value = $2, is_primary = $3, updated_at = NOW() WHERE id = $4",
		req.Label, req.LocationValue, req.IsPrimary, id)

	if err != nil {
		h.logger.Error("Error updating location", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Update failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Location updated"})
}

// DELETE /locations/:id
func (h *LocationHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	_, err := h.db.Exec(c.Request.Context(), "DELETE FROM client_locations WHERE id = $1", id)
	if err != nil {
		h.logger.Error("Error deleting location", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Delete failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Location deleted successfully"})
}
