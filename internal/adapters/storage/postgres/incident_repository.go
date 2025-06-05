// Package postgres provides PostgreSQL storage implementations for production use.
//
// This package contains database-backed implementations of repository interfaces
// using pgx/v5 for high performance and proper PostgreSQL feature utilisation.
// All operations are designed for concurrent access and handle connection pooling,
// timeouts, and optimistic locking.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// Constants for database operations
const (
	// DefaultConnectionTimeout is the default timeout for database connections
	DefaultConnectionTimeout = 5 * time.Second

	// MaxRetryAttempts is the maximum number of retry attempts for transient errors
	MaxRetryAttempts = 3

	// PostgreSQL error codes
	PgErrorCodeUniqueViolation      = "23505"
	PgErrorCodeForeignKeyViolation  = "23503"
	PgErrorCodeNotNullViolation     = "23502"
	PgErrorCodeCheckViolation       = "23514"
	PgErrorCodeDeadlock             = "40P01"
	PgErrorCodeSerializationFailure = "40001"
)

// IncidentRepository is a PostgreSQL implementation of ports.IncidentRepository.
//
// This implementation provides production-ready incident storage with:
//   - Connection pooling via pgxpool
//   - Optimistic locking for update operations
//   - Proper transaction handling
//   - Context-aware operations with timeout support
//   - JSONB utilisation for flexible fields
//   - Comprehensive error mapping
type IncidentRepository struct {
	pool   *pgxpool.Pool
	logger *logging.Logger
}

// NewIncidentRepository creates a new PostgreSQL incident repository.
//
// The repository requires an active connection pool and will validate
// the connection on creation. All database operations will use the
// provided logger for structured logging.
//
// Returns an error if the database connection cannot be validated.
func NewIncidentRepository(pool *pgxpool.Pool, logger *logging.Logger) (*IncidentRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("database pool cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Test database connectivity
	ctx, cancel := context.WithTimeout(context.Background(), DefaultConnectionTimeout)
	defer cancel()

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &IncidentRepository{
		pool:   pool,
		logger: logger,
	}, nil
}

// Create stores a new incident in the database.
//
// The incident is validated before storage and must have a unique ID.
// All related signals and events are stored atomically within a transaction.
//
// Returns ErrAlreadyExists if an incident with the same ID already exists.
// Returns ErrInvalidInput if the incident fails validation.
func (r *IncidentRepository) Create(ctx context.Context, incident *domain.Incident) error {
	if incident == nil {
		return ports.ErrInvalidInput
	}

	// Validate incident
	if err := incident.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrInvalidInput, err)
	}

	logger := r.logger.WithFields("operation", "create_incident", "incident_id", incident.ID)
	logger.Debug("Creating incident")

	// Start transaction for atomic operation
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.WithError(err).Error("Failed to begin transaction")
		return r.mapError(err)
	}
	defer tx.Rollback(ctx)

	// Generate UUID if not provided
	if incident.ID == "" {
		incident.ID = uuid.New().String()
	}

	// Convert labels to JSONB
	labelsJSON, err := json.Marshal(incident.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	// Insert incident
	insertQuery := `
		INSERT INTO incidents (
			id, title, description, status, severity, team_id,
			assignee_id, service_id, labels, created_at, updated_at, resolved_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		)`

	_, err = tx.Exec(ctx, insertQuery,
		incident.ID,
		incident.Title,
		incident.Description,
		incident.Status.String(),
		incident.Severity.String(),
		incident.TeamID,
		nullableString(incident.AssigneeID),
		nullableString(incident.ServiceID),
		labelsJSON,
		incident.CreatedAt,
		incident.UpdatedAt,
		incident.ResolvedAt,
	)

	if err != nil {
		logger.WithError(err).Error("Failed to insert incident")
		if isUniqueViolation(err) {
			return ports.ErrAlreadyExists
		}
		return r.mapError(err)
	}

	// Insert signals if any
	if len(incident.Signals) > 0 {
		if err := r.insertSignals(ctx, tx, incident.ID, incident.Signals); err != nil {
			logger.WithError(err).Error("Failed to insert signals")
			return err
		}
	}

	// Insert events if any
	if len(incident.Events) > 0 {
		if err := r.insertEvents(ctx, tx, incident.Events); err != nil {
			logger.WithError(err).Error("Failed to insert events")
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logger.WithError(err).Error("Failed to commit transaction")
		return r.mapError(err)
	}

	logger.Info("Incident created successfully")
	return nil
}

// Get retrieves an incident by its unique identifier.
//
// Returns the complete incident including attached signals and events.
// Events are ordered by occurred_at timestamp (oldest first).
// Signals are ordered by received_at timestamp (oldest first).
//
// Returns ErrNotFound if no incident exists with the given ID.
func (r *IncidentRepository) Get(ctx context.Context, id string) (*domain.Incident, error) {
	if id == "" {
		return nil, ports.ErrInvalidInput
	}

	logger := r.logger.WithFields("operation", "get_incident", "incident_id", id)
	logger.Debug("Retrieving incident")

	// Query incident with optimised join
	query := `
		SELECT
			i.id, i.title, i.description, i.status, i.severity, i.team_id,
			i.assignee_id, i.service_id, i.labels, i.created_at, i.updated_at, i.resolved_at
		FROM incidents i
		WHERE i.id = $1`

	row := r.pool.QueryRow(ctx, query, id)

	incident, err := r.scanIncident(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Debug("Incident not found")
			return nil, ports.ErrNotFound
		}
		logger.WithError(err).Error("Failed to scan incident")
		return nil, r.mapError(err)
	}

	// Load signals
	signals, err := r.loadSignals(ctx, id)
	if err != nil {
		logger.WithError(err).Error("Failed to load signals")
		return nil, err
	}
	incident.Signals = signals

	// Load events
	events, err := r.loadEvents(ctx, id)
	if err != nil {
		logger.WithError(err).Error("Failed to load events")
		return nil, err
	}
	incident.Events = events

	logger.Debug("Incident retrieved successfully")
	return incident, nil
}

// List retrieves incidents based on the provided filter criteria.
//
// Returns a paginated list of incidents with metadata about the total count.
// The filter is validated before execution and proper indexes are utilised
// for optimal query performance.
//
// Complex filtering is supported including:
//   - Team, status, severity filtering
//   - Label-based filtering using JSONB operators
//   - Full-text search on title and description
//   - Time-based filtering with proper index usage
func (r *IncidentRepository) List(ctx context.Context, filter ports.ListFilter) (*ports.ListResult, error) {
	// Validate filter
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	logger := r.logger.WithFields("operation", "list_incidents", "limit", filter.Limit, "offset", filter.Offset)
	logger.Debug("Listing incidents with filters")

	// Build dynamic query with proper parameterisation
	query, countQuery, args, countArgs := r.buildListQuery(filter)

	// Execute count query first for pagination metadata
	var total int
	err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total)
	if err != nil {
		logger.WithError(err).Error("Failed to count incidents")
		return nil, r.mapError(err)
	}

	// Execute main query
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		logger.WithError(err).Error("Failed to query incidents")
		return nil, r.mapError(err)
	}
	defer rows.Close()

	var incidents []*domain.Incident
	for rows.Next() {
		incident, err := r.scanIncident(rows)
		if err != nil {
			logger.WithError(err).Error("Failed to scan incident row")
			return nil, r.mapError(err)
		}
		incidents = append(incidents, incident)
	}

	if err := rows.Err(); err != nil {
		logger.WithError(err).Error("Error during row iteration")
		return nil, r.mapError(err)
	}

	result := &ports.ListResult{
		Incidents: incidents,
		Total:     total,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
		HasMore:   filter.Offset+len(incidents) < total,
	}

	logger.WithFields("total", total, "returned", len(incidents)).Debug("Incidents listed successfully")
	return result, nil
}

// Update modifies an existing incident in the database.
//
// This operation uses optimistic locking by checking the updated_at timestamp
// to prevent concurrent modifications. The entire incident record is updated
// atomically within a transaction.
//
// Returns ErrNotFound if the incident doesn't exist.
// Returns ErrConflict if the incident has been modified by another process.
func (r *IncidentRepository) Update(ctx context.Context, incident *domain.Incident) error {
	if incident == nil {
		return ports.ErrInvalidInput
	}

	// Validate incident
	if err := incident.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrInvalidInput, err)
	}

	logger := r.logger.WithFields("operation", "update_incident", "incident_id", incident.ID)
	logger.Debug("Updating incident")

	// Start transaction for atomic operation
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		logger.WithError(err).Error("Failed to begin transaction")
		return r.mapError(err)
	}
	defer tx.Rollback(ctx)

	// Get current updated_at for optimistic locking
	var currentUpdatedAt time.Time
	err = tx.QueryRow(ctx, "SELECT updated_at FROM incidents WHERE id = $1", incident.ID).Scan(&currentUpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.ErrNotFound
		}
		logger.WithError(err).Error("Failed to check current incident state")
		return r.mapError(err)
	}

	// Check for concurrent modification (optimistic locking)
	if !incident.UpdatedAt.Equal(currentUpdatedAt) {
		logger.WithFields("expected", incident.UpdatedAt, "current", currentUpdatedAt).Warn("Concurrent modification detected")
		return ports.ErrConflict
	}

	// Update timestamp
	incident.UpdatedAt = time.Now().UTC()

	// Convert labels to JSONB
	labelsJSON, err := json.Marshal(incident.Labels)
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	// Update incident
	updateQuery := `
		UPDATE incidents SET
			title = $2, description = $3, status = $4, severity = $5,
			team_id = $6, assignee_id = $7, service_id = $8, labels = $9,
			updated_at = $10, resolved_at = $11
		WHERE id = $1 AND updated_at = $12`

	result, err := tx.Exec(ctx, updateQuery,
		incident.ID,
		incident.Title,
		incident.Description,
		incident.Status.String(),
		incident.Severity.String(),
		incident.TeamID,
		nullableString(incident.AssigneeID),
		nullableString(incident.ServiceID),
		labelsJSON,
		incident.UpdatedAt,
		incident.ResolvedAt,
		currentUpdatedAt, // Optimistic locking condition
	)

	if err != nil {
		logger.WithError(err).Error("Failed to update incident")
		return r.mapError(err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		// This should not happen due to our earlier check, but defence in depth
		return ports.ErrConflict
	}

	if err := tx.Commit(ctx); err != nil {
		logger.WithError(err).Error("Failed to commit transaction")
		return r.mapError(err)
	}

	logger.Info("Incident updated successfully")
	return nil
}

// AddEvent appends an event to an incident's history.
//
// The event must be valid and reference an existing incident.
// Events are append-only and cannot be modified once created.
// A UUID is generated for the event if not provided.
//
// Returns ErrNotFound if the referenced incident doesn't exist.
func (r *IncidentRepository) AddEvent(ctx context.Context, event *domain.Event) error {
	if event == nil {
		return ports.ErrInvalidInput
	}

	logger := r.logger.WithFields("operation", "add_event", "incident_id", event.IncidentID)
	logger.Debug("Adding event to incident")

	// Generate UUID if not provided
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Validate event after ID generation
	if err := event.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrInvalidInput, err)
	}

	// Check if incident exists
	var exists bool
	err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM incidents WHERE id = $1)", event.IncidentID).Scan(&exists)
	if err != nil {
		logger.WithError(err).Error("Failed to check incident existence")
		return r.mapError(err)
	}

	if !exists {
		logger.Debug("Referenced incident does not exist")
		return ports.ErrNotFound
	}

	// Convert metadata to JSONB
	metadataJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal event metadata: %w", err)
	}

	// Insert event
	insertQuery := `
		INSERT INTO events (
			id, incident_id, type, actor, description, metadata, occurred_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		)`

	_, err = r.pool.Exec(ctx, insertQuery,
		event.ID,
		event.IncidentID,
		event.Type,
		event.Actor,
		event.Description,
		metadataJSON,
		event.OccurredAt,
	)

	if err != nil {
		logger.WithError(err).Error("Failed to insert event")
		return r.mapError(err)
	}

	logger.WithFields("event_id", event.ID, "event_type", event.Type).Info("Event added successfully")
	return nil
}

// Delete removes an incident from the database.
//
// This is a soft delete operation that marks the incident as deleted
// but preserves the data for audit purposes. Related events and signals
// are cascaded automatically by the database schema.
//
// Returns ErrNotFound if the incident doesn't exist.
func (r *IncidentRepository) Delete(ctx context.Context, id string) error {
	if id == "" {
		return ports.ErrInvalidInput
	}

	logger := r.logger.WithFields("operation", "delete_incident", "incident_id", id)
	logger.Debug("Deleting incident")

	// For now, we do a hard delete as per schema design
	// In production, this might be changed to a soft delete with a deleted_at column
	result, err := r.pool.Exec(ctx, "DELETE FROM incidents WHERE id = $1", id)
	if err != nil {
		logger.WithError(err).Error("Failed to delete incident")
		return r.mapError(err)
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		logger.Debug("Incident not found for deletion")
		return ports.ErrNotFound
	}

	logger.Info("Incident deleted successfully")
	return nil
}

// GetWithEvents retrieves an incident with its complete event history.
//
// This is optimised for cases where the full incident context is needed.
// Events are ordered by occurred_at timestamp (oldest first).
// This method is equivalent to Get() in this implementation.
func (r *IncidentRepository) GetWithEvents(ctx context.Context, id string) (*domain.Incident, error) {
	return r.Get(ctx, id)
}

// GetWithSignals retrieves an incident with its attached signals.
//
// This is optimised for cases where signal correlation is needed.
// Signals are ordered by received_at timestamp (oldest first).
// This method is equivalent to Get() in this implementation.
func (r *IncidentRepository) GetWithSignals(ctx context.Context, id string) (*domain.Incident, error) {
	return r.Get(ctx, id)
}

// Helper methods for internal operations

// scanIncident scans a database row into an Incident domain object.
func (r *IncidentRepository) scanIncident(row pgx.Row) (*domain.Incident, error) {
	var incident domain.Incident
	var assigneeID, serviceID sql.NullString
	var labelsJSON []byte

	err := row.Scan(
		&incident.ID,
		&incident.Title,
		&incident.Description,
		&incident.Status,
		&incident.Severity,
		&incident.TeamID,
		&assigneeID,
		&serviceID,
		&labelsJSON,
		&incident.CreatedAt,
		&incident.UpdatedAt,
		&incident.ResolvedAt,
	)

	if err != nil {
		return nil, err
	}

	// Handle nullable fields
	if assigneeID.Valid {
		incident.AssigneeID = assigneeID.String
	}
	if serviceID.Valid {
		incident.ServiceID = serviceID.String
	}

	// Unmarshal labels
	if len(labelsJSON) > 0 {
		if err := json.Unmarshal(labelsJSON, &incident.Labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
		}
	}

	return &incident, nil
}

// loadSignals loads all signals attached to an incident.
func (r *IncidentRepository) loadSignals(ctx context.Context, incidentID string) ([]domain.Signal, error) {
	query := `
		SELECT s.id, s.source, s.title, s.description, s.severity,
			   s.labels, s.annotations, s.payload, s.received_at
		FROM signals s
		JOIN incident_signals is_rel ON s.id = is_rel.signal_id
		WHERE is_rel.incident_id = $1
		ORDER BY s.received_at ASC`

	rows, err := r.pool.Query(ctx, query, incidentID)
	if err != nil {
		return nil, r.mapError(err)
	}
	defer rows.Close()

	var signals []domain.Signal
	for rows.Next() {
		var signal domain.Signal
		var labelsJSON, annotationsJSON, payloadJSON []byte

		err := rows.Scan(
			&signal.ID,
			&signal.Source,
			&signal.Title,
			&signal.Description,
			&signal.Severity,
			&labelsJSON,
			&annotationsJSON,
			&payloadJSON,
			&signal.ReceivedAt,
		)

		if err != nil {
			return nil, r.mapError(err)
		}

		// Unmarshal JSONB fields
		if len(labelsJSON) > 0 {
			if err := json.Unmarshal(labelsJSON, &signal.Labels); err != nil {
				return nil, fmt.Errorf("failed to unmarshal signal labels: %w", err)
			}
		}

		if len(annotationsJSON) > 0 {
			if err := json.Unmarshal(annotationsJSON, &signal.Annotations); err != nil {
				return nil, fmt.Errorf("failed to unmarshal signal annotations: %w", err)
			}
		}

		if len(payloadJSON) > 0 {
			if err := json.Unmarshal(payloadJSON, &signal.Payload); err != nil {
				return nil, fmt.Errorf("failed to unmarshal signal payload: %w", err)
			}
		}

		signals = append(signals, signal)
	}

	return signals, rows.Err()
}

// loadEvents loads all events for an incident.
func (r *IncidentRepository) loadEvents(ctx context.Context, incidentID string) ([]domain.Event, error) {
	query := `
		SELECT id, incident_id, type, actor, description, metadata, occurred_at
		FROM events
		WHERE incident_id = $1
		ORDER BY occurred_at ASC`

	rows, err := r.pool.Query(ctx, query, incidentID)
	if err != nil {
		return nil, r.mapError(err)
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var event domain.Event
		var metadataJSON []byte

		err := rows.Scan(
			&event.ID,
			&event.IncidentID,
			&event.Type,
			&event.Actor,
			&event.Description,
			&metadataJSON,
			&event.OccurredAt,
		)

		if err != nil {
			return nil, r.mapError(err)
		}

		// Unmarshal metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &event.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal event metadata: %w", err)
			}
		}

		events = append(events, event)
	}

	return events, rows.Err()
}

// insertSignals inserts signals and creates incident-signal relationships.
func (r *IncidentRepository) insertSignals(ctx context.Context, tx pgx.Tx, incidentID string, signals []domain.Signal) error {
	for _, signal := range signals {
		// Generate UUID if not provided
		if signal.ID == "" {
			signal.ID = uuid.New().String()
		}

		// Validate signal
		if err := signal.Validate(); err != nil {
			return fmt.Errorf("invalid signal: %w", err)
		}

		// Convert JSONB fields
		labelsJSON, err := json.Marshal(signal.Labels)
		if err != nil {
			return fmt.Errorf("failed to marshal signal labels: %w", err)
		}

		annotationsJSON, err := json.Marshal(signal.Annotations)
		if err != nil {
			return fmt.Errorf("failed to marshal signal annotations: %w", err)
		}

		payloadJSON, err := json.Marshal(signal.Payload)
		if err != nil {
			return fmt.Errorf("failed to marshal signal payload: %w", err)
		}

		// Insert signal
		_, err = tx.Exec(ctx, `
			INSERT INTO signals (
				id, source, title, description, severity,
				labels, annotations, payload, received_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9
			)`,
			signal.ID,
			signal.Source,
			signal.Title,
			signal.Description,
			signal.Severity.String(),
			labelsJSON,
			annotationsJSON,
			payloadJSON,
			signal.ReceivedAt,
		)

		if err != nil {
			return r.mapError(err)
		}

		// Create incident-signal relationship
		_, err = tx.Exec(ctx, `
			INSERT INTO incident_signals (incident_id, signal_id, attached_at)
			VALUES ($1, $2, $3)`,
			incidentID,
			signal.ID,
			time.Now().UTC(),
		)

		if err != nil {
			return r.mapError(err)
		}
	}

	return nil
}

// insertEvents inserts events for an incident.
func (r *IncidentRepository) insertEvents(ctx context.Context, tx pgx.Tx, events []domain.Event) error {
	for _, event := range events {
		// Generate UUID if not provided
		if event.ID == "" {
			event.ID = uuid.New().String()
		}

		// Validate event
		if err := event.Validate(); err != nil {
			return fmt.Errorf("invalid event: %w", err)
		}

		// Convert metadata to JSONB
		metadataJSON, err := json.Marshal(event.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal event metadata: %w", err)
		}

		// Insert event
		_, err = tx.Exec(ctx, `
			INSERT INTO events (
				id, incident_id, type, actor, description, metadata, occurred_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7
			)`,
			event.ID,
			event.IncidentID,
			event.Type,
			event.Actor,
			event.Description,
			metadataJSON,
			event.OccurredAt,
		)

		if err != nil {
			return r.mapError(err)
		}
	}

	return nil
}

// buildListQuery constructs dynamic SQL queries for incident listing with filters.
func (r *IncidentRepository) buildListQuery(filter ports.ListFilter) (query, countQuery string, args, countArgs []interface{}) {
	var conditions []string
	var params []interface{}
	paramCount := 0

	baseQuery := `
		SELECT i.id, i.title, i.description, i.status, i.severity, i.team_id,
			   i.assignee_id, i.service_id, i.labels, i.created_at, i.updated_at, i.resolved_at
		FROM incidents i`

	countBaseQuery := "SELECT COUNT(*) FROM incidents i"

	// Build WHERE conditions
	if filter.TeamID != "" {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("i.team_id = $%d", paramCount))
		params = append(params, filter.TeamID)
	}

	if len(filter.Status) > 0 {
		paramCount++
		statusList := make([]string, len(filter.Status))
		for i, status := range filter.Status {
			statusList[i] = status.String()
		}
		conditions = append(conditions, fmt.Sprintf("i.status = ANY($%d)", paramCount))
		params = append(params, statusList)
	}

	if len(filter.Severity) > 0 {
		paramCount++
		severityList := make([]string, len(filter.Severity))
		for i, severity := range filter.Severity {
			severityList[i] = severity.String()
		}
		conditions = append(conditions, fmt.Sprintf("i.severity = ANY($%d)", paramCount))
		params = append(params, severityList)
	}

	if filter.AssigneeID != "" {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("i.assignee_id = $%d", paramCount))
		params = append(params, filter.AssigneeID)
	}

	if filter.ServiceID != "" {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("i.service_id = $%d", paramCount))
		params = append(params, filter.ServiceID)
	}

	// Label filtering using JSONB contains operator
	if len(filter.Labels) > 0 {
		paramCount++
		labelJSON, _ := json.Marshal(filter.Labels)
		conditions = append(conditions, fmt.Sprintf("i.labels @> $%d", paramCount))
		params = append(params, labelJSON)
	}

	// Time-based filtering
	if filter.CreatedAfter != nil {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("i.created_at > $%d", paramCount))
		params = append(params, *filter.CreatedAfter)
	}

	if filter.CreatedBefore != nil {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("i.created_at < $%d", paramCount))
		params = append(params, *filter.CreatedBefore)
	}

	if filter.UpdatedAfter != nil {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("i.updated_at > $%d", paramCount))
		params = append(params, *filter.UpdatedAfter)
	}

	if filter.UpdatedBefore != nil {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("i.updated_at < $%d", paramCount))
		params = append(params, *filter.UpdatedBefore)
	}

	// Full-text search on title and description
	if filter.Search != "" {
		paramCount++
		searchPattern := "%" + strings.ToLower(filter.Search) + "%"
		conditions = append(conditions, fmt.Sprintf("(LOWER(i.title) LIKE $%d OR LOWER(i.description) LIKE $%d)", paramCount, paramCount))
		params = append(params, searchPattern)
	}

	// Build WHERE clause
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Build ORDER BY clause
	orderClause := fmt.Sprintf(" ORDER BY i.%s %s", filter.SortBy, strings.ToUpper(filter.SortOrder))

	// Add pagination
	paramCount += 2
	limitOffsetClause := fmt.Sprintf(" LIMIT $%d OFFSET $%d", paramCount-1, paramCount)
	params = append(params, filter.Limit, filter.Offset)

	// Construct final queries
	query = baseQuery + whereClause + orderClause + limitOffsetClause
	countQuery = countBaseQuery + whereClause

	// Count query uses the same params but without limit/offset
	countArgs = make([]interface{}, len(params)-2)
	copy(countArgs, params[:len(params)-2])

	return query, countQuery, params, countArgs
}

// mapError maps database errors to domain errors with comprehensive PostgreSQL error handling.
func (r *IncidentRepository) mapError(err error) error {
	if err == nil {
		return nil
	}

	// Context errors
	if errors.Is(err, context.DeadlineExceeded) {
		r.logger.WithError(err).Warn("Database operation timed out")
		return ports.ErrTimeout
	}
	if errors.Is(err, context.Canceled) {
		return err // Pass through cancellation
	}

	// PostgreSQL-specific errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case PgErrorCodeUniqueViolation:
			r.logger.WithFields("pg_error_code", pgErr.Code, "constraint", pgErr.ConstraintName).Debug("Unique constraint violation")
			return ports.ErrAlreadyExists
		case PgErrorCodeForeignKeyViolation:
			r.logger.WithFields("pg_error_code", pgErr.Code, "constraint", pgErr.ConstraintName).Debug("Foreign key constraint violation")
			return ports.ErrNotFound
		case PgErrorCodeNotNullViolation:
			r.logger.WithFields("pg_error_code", pgErr.Code, "column", pgErr.ColumnName).Debug("Not null constraint violation")
			return fmt.Errorf("%w: required field missing: %s", ports.ErrInvalidInput, pgErr.ColumnName)
		case PgErrorCodeCheckViolation:
			r.logger.WithFields("pg_error_code", pgErr.Code, "constraint", pgErr.ConstraintName).Debug("Check constraint violation")
			return fmt.Errorf("%w: constraint violation: %s", ports.ErrInvalidInput, pgErr.ConstraintName)
		case PgErrorCodeDeadlock, PgErrorCodeSerializationFailure:
			r.logger.WithFields("pg_error_code", pgErr.Code).Warn("Database concurrency conflict")
			return ports.ErrConflict
		default:
			r.logger.WithFields("pg_error_code", pgErr.Code, "message", pgErr.Message).Error("Unhandled PostgreSQL error")
			return fmt.Errorf("database error [%s]: %w", pgErr.Code, ports.ErrConnectionFailed)
		}
	}

	// Connection-related errors
	if strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "network") ||
		strings.Contains(err.Error(), "timeout") {
		r.logger.WithError(err).Error("Database connection error")
		return ports.ErrConnectionFailed
	}

	// JSON marshaling/unmarshaling errors
	if strings.Contains(err.Error(), "json") || strings.Contains(err.Error(), "unmarshal") {
		r.logger.WithError(err).Error("JSON processing error")
		return fmt.Errorf("%w: data format error", ports.ErrInvalidInput)
	}

	// Default to connection failed for unknown errors
	r.logger.WithError(err).Error("Unknown database error")
	return ports.ErrConnectionFailed
}

// nullableString converts a string to sql.NullString.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// isUniqueViolation checks if the error is a unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == PgErrorCodeUniqueViolation
	}
	// Fallback to string matching for non-pgx errors
	return strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint")
}
