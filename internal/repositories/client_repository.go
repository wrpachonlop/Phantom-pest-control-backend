package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/phantompestcontrol/crm/internal/dto"
	"github.com/phantompestcontrol/crm/internal/models"
)

// ClientRepository handles all DB operations for clients.
type ClientRepository struct {
	db *pgxpool.Pool
}

func NewClientRepository(db *pgxpool.Pool) *ClientRepository {
	return &ClientRepository{db: db}
}

// GetByID returns a single client with all related data.
func (r *ClientRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.ClientFull, error) {
	client := &models.ClientFull{}

	client.ContactMethod = &models.ContactMethod{}

	err := r.db.QueryRow(ctx, `
		SELECT
			c.id, c.client_name, c.client_type, c.property_type, c.status,
			c.client_contact_date, c.first_contact_date, c.sold_date,
			c.after_hours, c.contact_method_id, c.problem_description,
			c.location_type, c.location_value, c.sold_by, c.sale_range,
			c.created_by, c.created_at, c.updated_at,
			cm.id, cm.name, cm.is_active, cm.created_at
		FROM clients c
		JOIN contact_methods cm ON cm.id = c.contact_method_id
		WHERE c.id = $1
	`, id).Scan(
		&client.ID, &client.ClientName, &client.ClientType, &client.PropertyType, &client.Status,
		&client.ClientContactDate, &client.FirstContactDate, &client.SoldDate,
		&client.AfterHours, &client.ContactMethodID, &client.ProblemDescription,
		&client.LocationType, &client.LocationValue, &client.SoldBy, &client.SaleRange,
		&client.CreatedBy, &client.CreatedAt, &client.UpdatedAt,
		// contact method
		&client.ContactMethod.ID, &client.ContactMethod.Name,
		&client.ContactMethod.IsActive, &client.ContactMethod.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get client by id: %w", err)
	}

	// Load phones
	phones, err := r.getPhones(ctx, id)
	if err != nil {
		return nil, err
	}
	client.Phones = phones

	// Load emails
	emails, err := r.getEmails(ctx, id)
	if err != nil {
		return nil, err
	}
	client.Emails = emails

	// Load pest issues
	pestIssues, err := r.getPestIssues(ctx, id)
	if err != nil {
		return nil, err
	}
	client.PestIssues = pestIssues

	return client, nil
}

// List returns paginated clients with optional filters.
func (r *ClientRepository) List(ctx context.Context, req *dto.ClientListRequest) ([]*models.Client, int64, error) {
	where := []string{"1=1"}
	args := []interface{}{}
	argIdx := 1

	if req.Status != nil {
		where = append(where, fmt.Sprintf("c.status = $%d", argIdx))
		args = append(args, *req.Status)
		argIdx++
	}
	if req.PropertyType != nil {
		where = append(where, fmt.Sprintf("c.property_type = $%d", argIdx))
		args = append(args, *req.PropertyType)
		argIdx++
	}
	if req.AfterHours != nil {
		where = append(where, fmt.Sprintf("c.after_hours = $%d", argIdx))
		args = append(args, *req.AfterHours)
		argIdx++
	}
	if req.Search != "" {
		// pattern es el texto que el usuario escribió (ej: "778")
		pattern := "%" + req.Search + "%"

		condition := fmt.Sprintf(`(
        c.client_name ILIKE $%d 
        OR c.location_value ILIKE $%d 
        OR EXISTS (
            SELECT 1 FROM phones p 
            WHERE p.client_id = c.id 
            AND p.phone_number ILIKE $%d
        )
    	)`, argIdx, argIdx+1, argIdx+2)

		// Usamos EXISTS para buscar en la tabla relacionada 'phones'
		where = append(where, condition)

		// Añadimos el patrón tres veces a los argumentos
		args = append(args, pattern, pattern, pattern)
		argIdx += 3
	}
	if req.DateFrom != "" {
		where = append(where, fmt.Sprintf("c.client_contact_date >= $%d", argIdx))
		args = append(args, req.DateFrom)
		argIdx++
	}
	if req.DateTo != "" {
		where = append(where, fmt.Sprintf("c.client_contact_date <= $%d", argIdx))
		args = append(args, req.DateTo)
		argIdx++
	}

	whereClause := strings.Join(where, " AND ")

	// Count total
	var total int64
	err := r.db.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM clients c WHERE %s", whereClause),
		args...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count clients: %w", err)
	}

	// Sort
	sortBy := "c.created_at"
	if req.SortBy != "" {
		allowed := map[string]string{
			"created_at":          "c.created_at",
			"client_contact_date": "c.client_contact_date",
			"first_contact_date":  "c.first_contact_date",
			"status":              "c.status",
			"client_name":         "c.client_name",
		}
		if col, ok := allowed[req.SortBy]; ok {
			sortBy = col
		}
	}
	sortDir := "DESC"
	if strings.ToUpper(req.SortDir) == "ASC" {
		sortDir = "ASC"
	}

	// Pagination
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 25
	}
	offset := (req.Page - 1) * req.PageSize

	args = append(args, req.PageSize, offset)
	query := fmt.Sprintf(`
		SELECT
			c.id, c.client_name, c.client_type, c.property_type, c.status,
			c.client_contact_date, c.first_contact_date, c.sold_date,
			c.after_hours, c.contact_method_id, c.problem_description,
			c.location_type, c.location_value, c.sold_by, c.sale_range,
			c.created_by, c.created_at, c.updated_at
		FROM clients c
		WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, whereClause, sortBy, sortDir, argIdx, argIdx+1)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	clients := make([]*models.Client, 0)
	for rows.Next() {
		c := &models.Client{}
		if err := rows.Scan(
			&c.ID, &c.ClientName, &c.ClientType, &c.PropertyType, &c.Status,
			&c.ClientContactDate, &c.FirstContactDate, &c.SoldDate,
			&c.AfterHours, &c.ContactMethodID, &c.ProblemDescription,
			&c.LocationType, &c.LocationValue, &c.SoldBy, &c.SaleRange,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan client: %w", err)
		}
		clients = append(clients, c)
	}

	return clients, total, nil
}

// Create inserts a new client and related records atomically.
func (r *ClientRepository) Create(
	ctx context.Context,
	client *models.Client,
	phones []models.Phone,
	emails []models.Email,
	pestIssueIDs []uuid.UUID,
) (*models.Client, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert client
	err = tx.QueryRow(ctx, `
		INSERT INTO clients (
			client_name, client_type, property_type, status,
			client_contact_date, after_hours, contact_method_id,
			problem_description, location_type, location_value,
			sold_by, sale_range, sold_date, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, created_at, updated_at
	`,
		client.ClientName, client.ClientType, client.PropertyType, client.Status,
		client.ClientContactDate, client.AfterHours, client.ContactMethodID,
		client.ProblemDescription, client.LocationType, client.LocationValue,
		client.SoldBy, client.SaleRange, client.SoldDate, client.CreatedBy,
	).Scan(&client.ID, &client.CreatedAt, &client.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("insert client: %w", err)
	}

	// Insert phones
	for _, p := range phones {
		_, err = tx.Exec(ctx,
			`INSERT INTO phones (client_id, phone_number, label) VALUES ($1, $2, $3)`,
			client.ID, p.PhoneNumber, p.Label,
		)
		if err != nil {
			return nil, fmt.Errorf("insert phone: %w", err)
		}
	}

	// Insert emails
	for _, e := range emails {
		_, err = tx.Exec(ctx,
			`INSERT INTO emails (client_id, email, label) VALUES ($1, $2, $3)`,
			client.ID, e.Email, e.Label,
		)
		if err != nil {
			return nil, fmt.Errorf("insert email: %w", err)
		}
	}

	// Insert pest issues
	for _, pid := range pestIssueIDs {
		_, err = tx.Exec(ctx,
			`INSERT INTO client_pest_issues (client_id, pest_issue_id) VALUES ($1, $2)`,
			client.ID, pid,
		)
		if err != nil {
			return nil, fmt.Errorf("insert pest issue: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return client, nil
}

func (r *ClientRepository) syncPhonesTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, phones []dto.PhoneInput) error {
	_, err := tx.Exec(ctx, `DELETE FROM phones WHERE client_id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete phones: %w", err)
	}
	for _, p := range phones {
		_, err = tx.Exec(ctx,
			`INSERT INTO phones (client_id, phone_number, label) VALUES ($1, $2, $3)`,
			id, p.PhoneNumber, p.Label,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ClientRepository) syncEmailsTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, emails []dto.EmailInput) error {
	_, err := tx.Exec(ctx, `DELETE FROM emails WHERE client_id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete emails: %w", err)
	}
	for _, e := range emails {
		_, err = tx.Exec(ctx,
			`INSERT INTO emails (client_id, email, label) VALUES ($1, $2, $3)`,
			id, e.Email, e.Label,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ClientRepository) syncPestsTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, pests []uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM client_pest_issues WHERE client_id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete pests: %w", err)
	}
	for _, pid := range pests {
		_, err = tx.Exec(ctx,
			`INSERT INTO client_pest_issues (client_id, pest_issue_id) VALUES ($1, $2)`,
			id, pid,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// Update performs a partial update on a client.
func (r *ClientRepository) Update(
	ctx context.Context,
	id uuid.UUID,
	updates map[string]interface{},
	phones *[]dto.PhoneInput,
	emails *[]dto.EmailInput,
	pests *[]uuid.UUID,
) (*models.Client, error) {
	// 1. Iniciamos Transacción
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 2. Sincronizar listas relacionadas (Delete & Insert)
	// Usamos los helpers privados que definimos antes
	if phones != nil {
		if err := r.syncPhonesTx(ctx, tx, id, *phones); err != nil {
			return nil, err
		}
	}
	if emails != nil {
		if err := r.syncEmailsTx(ctx, tx, id, *emails); err != nil {
			return nil, err
		}
	}
	if pests != nil {
		if err := r.syncPestsTx(ctx, tx, id, *pests); err != nil {
			return nil, err
		}
	}

	// 3. Preparar la actualización de la tabla principal 'clients'
	c := &models.Client{}

	// Filtramos el mapa updates para no intentar insertar columnas que no existen en 'clients'
	// (Tu s.audit.Log añade 'pest_issues' al mapa, pero eso no es una columna de la tabla)
	validUpdates := map[string]interface{}{}
	for k, v := range updates {
		if k != "pest_issues" && k != "phones" && k != "emails" {
			validUpdates[k] = v
		}
	}

	if len(validUpdates) > 0 {
		setClauses := []string{}
		args := []interface{}{}
		argIdx := 1

		for col, val := range validUpdates {
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, val)
			argIdx++
		}

		args = append(args, id)
		query := fmt.Sprintf(`
            UPDATE clients SET %s, updated_at = NOW()
            WHERE id = $%d
            RETURNING id, client_name, client_type, property_type, status,
                client_contact_date, first_contact_date, sold_date,
                after_hours, contact_method_id, problem_description,
                location_type, location_value, sold_by, sale_range,
                created_by, created_at, updated_at
        `, strings.Join(setClauses, ", "), argIdx)

		// IMPORTANTE: Usamos tx.QueryRow, no r.db.QueryRow
		err = tx.QueryRow(ctx, query, args...).Scan(
			&c.ID, &c.ClientName, &c.ClientType, &c.PropertyType, &c.Status,
			&c.ClientContactDate, &c.FirstContactDate, &c.SoldDate,
			&c.AfterHours, &c.ContactMethodID, &c.ProblemDescription,
			&c.LocationType, &c.LocationValue, &c.SoldBy, &c.SaleRange,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("update client row: %w", err)
		}
	} else {
		// Si no hay cambios en la tabla principal, cargamos el cliente para el retorno
		// (Podemos usar tx para leer también)
		full, err := r.GetByID(ctx, id) // O una query simple aquí
		if err != nil {
			return nil, err
		}
		c = &full.Client
	}

	// 4. Confirmar cambios
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return c, nil
	// if len(updates) == 0 {
	// 	full, err := r.GetByID(ctx, id)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	// Retornamos la dirección de la estructura Client embebida
	// 	return &full.Client, nil
	// }

	// setClauses := []string{}
	// args := []interface{}{}
	// argIdx := 1

	// for col, val := range updates {
	// 	setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
	// 	args = append(args, val)
	// 	argIdx++
	// }

	// args = append(args, id)
	// query := fmt.Sprintf(`
	// 	UPDATE clients SET %s, updated_at = NOW()
	// 	WHERE id = $%d
	// 	RETURNING id, client_name, client_type, property_type, status,
	// 		client_contact_date, first_contact_date, sold_date,
	// 		after_hours, contact_method_id, problem_description,
	// 		location_type, location_value, sold_by, sale_range,
	// 		created_by, created_at, updated_at
	// `, strings.Join(setClauses, ", "), argIdx)

	// c := &models.Client{}
	// err := r.db.QueryRow(ctx, query, args...).Scan(
	// 	&c.ID, &c.ClientName, &c.ClientType, &c.PropertyType, &c.Status,
	// 	&c.ClientContactDate, &c.FirstContactDate, &c.SoldDate,
	// 	&c.AfterHours, &c.ContactMethodID, &c.ProblemDescription,
	// 	&c.LocationType, &c.LocationValue, &c.SoldBy, &c.SaleRange,
	// 	&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	// )
	// if err == pgx.ErrNoRows {
	// 	return nil, ErrNotFound
	// }
	// if err != nil {
	// 	return nil, fmt.Errorf("update client: %w", err)
	// }

	// return c, nil
}

// Delete removes a client (admin only – enforced at handler layer).
func (r *ClientRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, `DELETE FROM clients WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete client: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckDuplicates finds clients with matching phone numbers or emails.
func (r *ClientRepository) CheckDuplicates(ctx context.Context, phones, emails []string) ([]*models.Client, string, error) {
	if len(phones) == 0 && len(emails) == 0 {
		return nil, "", nil
	}

	type match struct {
		clientID uuid.UUID
		matchOn  string
	}
	matchedIDs := map[uuid.UUID]string{}

	// Check phone matches
	if len(phones) > 0 {
		rows, err := r.db.Query(ctx,
			`SELECT DISTINCT client_id FROM phones WHERE phone_number = ANY($1)`,
			phones,
		)
		if err != nil {
			return nil, "", err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			rows.Scan(&id)
			matchedIDs[id] = "phone"
		}
	}

	// Check email matches
	if len(emails) > 0 {
		rows, err := r.db.Query(ctx,
			`SELECT DISTINCT client_id FROM emails WHERE email = ANY($1)`,
			emails,
		)
		if err != nil {
			return nil, "", err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			rows.Scan(&id)
			if _, exists := matchedIDs[id]; exists {
				matchedIDs[id] = "both"
			} else {
				matchedIDs[id] = "email"
			}
		}
	}

	if len(matchedIDs) == 0 {
		return nil, "", nil
	}

	// Fetch matched clients
	ids := make([]uuid.UUID, 0, len(matchedIDs))
	for id := range matchedIDs {
		ids = append(ids, id)
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, client_name, client_type, property_type, status,
			client_contact_date, first_contact_date, sold_date,
			after_hours, contact_method_id, problem_description,
			location_type, location_value, sold_by, sale_range,
			created_by, created_at, updated_at
		FROM clients WHERE id = ANY($1)
	`, ids)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	clients := []*models.Client{}
	for rows.Next() {
		c := &models.Client{}
		rows.Scan(
			&c.ID, &c.ClientName, &c.ClientType, &c.PropertyType, &c.Status,
			&c.ClientContactDate, &c.FirstContactDate, &c.SoldDate,
			&c.AfterHours, &c.ContactMethodID, &c.ProblemDescription,
			&c.LocationType, &c.LocationValue, &c.SoldBy, &c.SaleRange,
			&c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		)
		clients = append(clients, c)
	}

	// Determine overall match reason
	matchOn := "phone"
	for _, v := range matchedIDs {
		if v == "both" {
			matchOn = "both"
			break
		}
		matchOn = v
	}

	return clients, matchOn, nil
}

// ─── Private helpers ──────────────────────────────────────────

func (r *ClientRepository) getPhones(ctx context.Context, clientID uuid.UUID) ([]models.Phone, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, client_id, phone_number, label, created_at FROM phones WHERE client_id = $1`,
		clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	phones := []models.Phone{}
	for rows.Next() {
		p := models.Phone{}
		rows.Scan(&p.ID, &p.ClientID, &p.PhoneNumber, &p.Label, &p.CreatedAt)
		phones = append(phones, p)
	}
	return phones, nil
}

func (r *ClientRepository) getEmails(ctx context.Context, clientID uuid.UUID) ([]models.Email, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, client_id, email, label, created_at FROM emails WHERE client_id = $1`,
		clientID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	emails := []models.Email{}
	for rows.Next() {
		e := models.Email{}
		rows.Scan(&e.ID, &e.ClientID, &e.Email, &e.Label, &e.CreatedAt)
		emails = append(emails, e)
	}
	return emails, nil
}

func (r *ClientRepository) getPestIssues(ctx context.Context, clientID uuid.UUID) ([]models.PestIssue, error) {
	rows, err := r.db.Query(ctx, `
		SELECT pi.id, pi.name, pi.is_active, pi.created_at
		FROM pest_issues pi
		JOIN client_pest_issues cpi ON cpi.pest_issue_id = pi.id
		WHERE cpi.client_id = $1
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	issues := []models.PestIssue{}
	for rows.Next() {
		pi := models.PestIssue{}
		rows.Scan(&pi.ID, &pi.Name, &pi.IsActive, &pi.CreatedAt)
		issues = append(issues, pi)
	}
	return issues, nil
}
func (r *ClientRepository) GetAuditLog(ctx context.Context, clientID uuid.UUID) ([]map[string]interface{}, error) {
	rows, err := r.db.Query(ctx, `
        SELECT 
            id, user_id, action, entity_type, entity_id, 
            old_values, new_values, host(ip_address), user_agent, created_at
        FROM audit_logs
        WHERE entity_id = $1 AND entity_type = 'client'
        ORDER BY created_at DESC
    `, clientID) // Usamos host(ip_address) para convertir el INET a string legible

	if err != nil {
		return nil, fmt.Errorf("query audit log: %w", err)
	}
	defer rows.Close()

	logs := []map[string]interface{}{}
	for rows.Next() {
		var id, userID, entityID uuid.UUID
		var action, entityType string
		var oldValues, newValues interface{}
		var ip, ua *string
		var createdAt time.Time

		err := rows.Scan(&id, &userID, &action, &entityType, &entityID, &oldValues, &newValues, &ip, &ua, &createdAt)
		if err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}

		logs = append(logs, map[string]interface{}{
			"id":          id,
			"user_id":     userID,
			"action":      action,
			"entity_type": entityType,
			"entity_id":   entityID,
			"old_values":  oldValues,
			"new_values":  newValues,
			"ip_address":  ip,
			"user_agent":  ua,
			"created_at":  createdAt,
		})
	}
	return logs, nil
}

// GetSnapshot returns a JSON-serializable snapshot of a client for audit purposes.
func (r *ClientRepository) GetSnapshot(ctx context.Context, id uuid.UUID) (map[string]interface{}, error) {
	client, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Convert to map for generic JSON storage
	data := map[string]interface{}{
		"id":                  client.ID,
		"client_name":         client.ClientName,
		"status":              client.Status,
		"property_type":       client.PropertyType,
		"client_contact_date": client.ClientContactDate.Format(time.DateOnly),
		"first_contact_date":  client.FirstContactDate,
		"sold_date":           client.SoldDate,
		"after_hours":         client.AfterHours,
		"contact_method_id":   client.ContactMethodID,
		"problem_description": client.ProblemDescription,
		"location_type":       client.LocationType,
		"location_value":      client.LocationValue,
		"sale_range":          client.SaleRange,
		"sold_by":             client.SoldBy,
	}
	return data, nil
}

// Sentinel errors
var ErrNotFound = fmt.Errorf("record not found")
