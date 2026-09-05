package repository

import (
	"database/sql"
	"fmt"

	"github.com/oronno/privateledger/internal/model"
)

// SICMappingRepository persists SIC-to-category mappings in SQLite.
type SICMappingRepository struct {
	db *sql.DB
}

// NewSICMappingRepository creates a SIC mapping repository.
func NewSICMappingRepository(db *sql.DB) *SICMappingRepository {
	return &SICMappingRepository{db: db}
}

// Count returns the number of stored mappings.
func (r *SICMappingRepository) Count() (int, error) {
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM sic_mapping").Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count SIC mappings: %w", err)
	}
	return count, nil
}

// Create inserts one mapping.
func (r *SICMappingRepository) Create(mapping *model.SICMapping) error {
	result, err := r.db.Exec(`
		INSERT INTO sic_mapping (sic_code, description, description_detail, category_id)
		VALUES (?, ?, ?, ?)
	`, mapping.SICCode, mapping.Description, mapping.DescriptionDetail, mapping.CategoryID)
	if err != nil {
		return fmt.Errorf("failed to create SIC mapping: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get SIC mapping ID: %w", err)
	}
	mapping.SICMappingID = int(id)
	return nil
}

// GetByID returns one mapping or nil when it does not exist.
func (r *SICMappingRepository) GetByID(id int) (*model.SICMapping, error) {
	return r.getOne(`
		SELECT sm.sic_mapping_id, sm.sic_code, sm.description, sm.description_detail,
			sm.category_id, sm.created_at, c.name
		FROM sic_mapping sm
		LEFT JOIN category c ON sm.category_id = c.category_id
		WHERE sm.sic_mapping_id = ?
	`, id)
}

// GetByCode returns one mapping by canonical SIC code or nil when absent.
func (r *SICMappingRepository) GetByCode(rawSICCode string) (*model.SICMapping, error) {
	sicCode, err := model.ParseSICCode(rawSICCode)
	if err != nil {
		return nil, fmt.Errorf("invalid SIC mapping lookup: %w", err)
	}
	return r.getOne(`
		SELECT sm.sic_mapping_id, sm.sic_code, sm.description, sm.description_detail,
			sm.category_id, sm.created_at, c.name
		FROM sic_mapping sm
		LEFT JOIN category c ON sm.category_id = c.category_id
		WHERE sm.sic_code = ?
	`, sicCode)
}

func (r *SICMappingRepository) getOne(query string, arg any) (*model.SICMapping, error) {
	var mapping model.SICMapping
	err := r.db.QueryRow(query, arg).Scan(
		&mapping.SICMappingID,
		&mapping.SICCode,
		&mapping.Description,
		&mapping.DescriptionDetail,
		&mapping.CategoryID,
		&mapping.CreatedAt,
		&mapping.CategoryName,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get SIC mapping: %w", err)
	}
	return &mapping, nil
}

// GetAll returns all mappings with optional category display data.
func (r *SICMappingRepository) GetAll() ([]*model.SICMapping, error) {
	rows, err := r.db.Query(`
		SELECT sm.sic_mapping_id, sm.sic_code, sm.description, sm.description_detail,
			sm.category_id, sm.created_at, c.name
		FROM sic_mapping sm
		LEFT JOIN category c ON sm.category_id = c.category_id
		ORDER BY sm.sic_code
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query SIC mappings: %w", err)
	}
	defer rows.Close()

	mappings := make([]*model.SICMapping, 0)
	for rows.Next() {
		var mapping model.SICMapping
		if err := rows.Scan(
			&mapping.SICMappingID,
			&mapping.SICCode,
			&mapping.Description,
			&mapping.DescriptionDetail,
			&mapping.CategoryID,
			&mapping.CreatedAt,
			&mapping.CategoryName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan SIC mapping: %w", err)
		}
		mappings = append(mappings, &mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating SIC mappings: %w", err)
	}
	return mappings, nil
}

// Update changes all user-editable mapping fields.
func (r *SICMappingRepository) Update(mapping *model.SICMapping) error {
	result, err := r.db.Exec(`
		UPDATE sic_mapping
		SET sic_code = ?, description = ?, description_detail = ?, category_id = ?
		WHERE sic_mapping_id = ?
	`, mapping.SICCode, mapping.Description, mapping.DescriptionDetail, mapping.CategoryID, mapping.SICMappingID)
	if err != nil {
		return fmt.Errorf("failed to update SIC mapping: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get updated SIC mapping count: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("SIC mapping not found")
	}
	return nil
}

// Delete removes one mapping without changing existing transaction categories.
func (r *SICMappingRepository) Delete(id int) error {
	result, err := r.db.Exec("DELETE FROM sic_mapping WHERE sic_mapping_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete SIC mapping: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get deleted SIC mapping count: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("SIC mapping not found")
	}
	return nil
}

// BulkInsertAtomic inserts a fully validated mapping set in one transaction.
func (r *SICMappingRepository) BulkInsertAtomic(mappings []*model.SICMapping) error {
	return r.withAtomicInsert(mappings, false)
}

// ReplaceAll atomically replaces the authoritative mapping set. UOW-2 owns
// validation, backup, cache reload, and recategorization around this primitive.
func (r *SICMappingRepository) ReplaceAll(mappings []*model.SICMapping) error {
	return r.withAtomicInsert(mappings, true)
}

func (r *SICMappingRepository) withAtomicInsert(mappings []*model.SICMapping, replace bool) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin SIC mapping transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if replace {
		if _, err := tx.Exec("DELETE FROM sic_mapping"); err != nil {
			return fmt.Errorf("failed to clear SIC mappings: %w", err)
		}
	}

	stmt, err := tx.Prepare(`
		INSERT INTO sic_mapping (sic_code, description, description_detail, category_id)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare SIC mapping insert: %w", err)
	}
	defer stmt.Close()

	for _, mapping := range mappings {
		if _, err := stmt.Exec(mapping.SICCode, mapping.Description, mapping.DescriptionDetail, mapping.CategoryID); err != nil {
			return fmt.Errorf("failed to insert SIC mapping: %w", err)
		}
	}
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("failed to close SIC mapping insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit SIC mappings: %w", err)
	}
	committed = true
	return nil
}
