package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

// LabelSeverity represents the severity level of a label.
type LabelSeverity string

const (
	SeverityInform   LabelSeverity = "inform"
	SeverityAlert    LabelSeverity = "alert"
	SeverityTakedown LabelSeverity = "takedown"
)

// LabelVisibility represents how labeled content should be displayed.
type LabelVisibility string

const (
	VisibilityIgnore LabelVisibility = "ignore"
	VisibilityShow   LabelVisibility = "show"
	VisibilityWarn   LabelVisibility = "warn"
	VisibilityHide   LabelVisibility = "hide"
)

// LabelDefinition represents a label type definition.
type LabelDefinition struct {
	Val               string
	Description       string
	Severity          LabelSeverity
	DefaultVisibility LabelVisibility
	CreatedAt         time.Time
}

// LabelDefinitionsRepository handles label definition persistence.
type LabelDefinitionsRepository struct {
	db database.Executor
}

// NewLabelDefinitionsRepository creates a new label definitions repository.
func NewLabelDefinitionsRepository(db database.Executor) *LabelDefinitionsRepository {
	return &LabelDefinitionsRepository{db: db}
}

// GetAll retrieves all label definitions.
func (r *LabelDefinitionsRepository) GetAll(ctx context.Context) ([]LabelDefinition, error) {
	sqlStr := "SELECT val, description, severity, default_visibility, created_at FROM label_definition ORDER BY val"

	rows, err := r.db.DB().QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanLabelDefinitions(rows)
}

// GetNonSystem retrieves all non-system label definitions (excludes labels starting with !).
func (r *LabelDefinitionsRepository) GetNonSystem(ctx context.Context) ([]LabelDefinition, error) {
	sqlStr := "SELECT val, description, severity, default_visibility, created_at FROM label_definition WHERE val NOT LIKE '!%' ORDER BY val"

	rows, err := r.db.DB().QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanLabelDefinitions(rows)
}

// Get retrieves a label definition by value.
func (r *LabelDefinitionsRepository) Get(ctx context.Context, val string) (*LabelDefinition, error) {
	sqlStr := fmt.Sprintf("SELECT val, description, severity, default_visibility, created_at FROM label_definition WHERE val = %s",
		r.db.Placeholder(1))

	var def LabelDefinition
	var createdAtStr string

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(val)},
		&def.Val, &def.Description, &def.Severity, &def.DefaultVisibility, &createdAtStr)
	if err != nil {
		return nil, err
	}

	def.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	if def.CreatedAt.IsZero() {
		def.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
	}

	return &def, nil
}

// Insert creates a new label definition.
func (r *LabelDefinitionsRepository) Insert(ctx context.Context, val, description string, severity LabelSeverity, defaultVisibility LabelVisibility) error {
	sqlStr := fmt.Sprintf(`INSERT INTO label_definition (val, description, severity, default_visibility)
		VALUES (%s, %s, %s, %s)`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4))

	params := []database.Value{
		database.Text(val),
		database.Text(description),
		database.Text(string(severity)),
		database.Text(string(defaultVisibility)),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// Exists checks if a label definition exists.
func (r *LabelDefinitionsRepository) Exists(ctx context.Context, val string) (bool, error) {
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM label_definition WHERE val = %s", r.db.Placeholder(1))

	var count int64
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(val)}, &count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}

// ValidateVisibility validates a visibility value.
func ValidateVisibility(visibility string) (LabelVisibility, error) {
	switch visibility {
	case "ignore":
		return VisibilityIgnore, nil
	case "show":
		return VisibilityShow, nil
	case "warn":
		return VisibilityWarn, nil
	case "hide":
		return VisibilityHide, nil
	default:
		return "", fmt.Errorf("invalid visibility: %s", visibility)
	}
}

// ValidateSeverity validates a severity value.
func ValidateSeverity(severity string) (LabelSeverity, error) {
	switch severity {
	case "inform":
		return SeverityInform, nil
	case "alert":
		return SeverityAlert, nil
	case "takedown":
		return SeverityTakedown, nil
	default:
		return "", fmt.Errorf("invalid severity: %s", severity)
	}
}

// Helper function to scan label definitions from rows
func scanLabelDefinitions(rows *sql.Rows) ([]LabelDefinition, error) {
	var definitions []LabelDefinition
	for rows.Next() {
		var def LabelDefinition
		var createdAtStr string

		if err := rows.Scan(&def.Val, &def.Description, &def.Severity, &def.DefaultVisibility, &createdAtStr); err != nil {
			return nil, err
		}

		def.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if def.CreatedAt.IsZero() {
			def.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		}
		definitions = append(definitions, def)
	}

	return definitions, rows.Err()
}
