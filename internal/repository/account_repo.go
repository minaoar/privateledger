package repository

import (
	"database/sql"
	"fmt"

	"github.com/oronno/privateledger/internal/model"
)

// AccountRepository handles database operations for accounts
type AccountRepository struct {
	db *sql.DB
}

// NewAccountRepository creates a new AccountRepository
func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

// Create inserts a new account into the database
func (r *AccountRepository) Create(account *model.Account) error {
	query := `INSERT INTO account (name) VALUES (?)`
	result, err := r.db.Exec(query, account.Name)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get account ID: %w", err)
	}

	account.AccountID = int(id)
	return nil
}

// GetByID retrieves an account by its ID
func (r *AccountRepository) GetByID(accountID int) (*model.Account, error) {
	query := `SELECT account_id, name, created_at FROM account WHERE account_id = ?`

	var account model.Account
	err := r.db.QueryRow(query, accountID).Scan(
		&account.AccountID,
		&account.Name,
		&account.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	return &account, nil
}

// GetByName retrieves an account by its name
func (r *AccountRepository) GetByName(name string) (*model.Account, error) {
	query := `SELECT account_id, name, created_at FROM account WHERE name = ?`

	var account model.Account
	err := r.db.QueryRow(query, name).Scan(
		&account.AccountID,
		&account.Name,
		&account.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get account by name: %w", err)
	}

	return &account, nil
}

// GetAll retrieves all accounts
func (r *AccountRepository) GetAll() ([]*model.Account, error) {
	query := `SELECT account_id, name, created_at FROM account ORDER BY name ASC`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*model.Account
	for rows.Next() {
		var account model.Account
		err := rows.Scan(
			&account.AccountID,
			&account.Name,
			&account.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts = append(accounts, &account)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating accounts: %w", err)
	}

	return accounts, nil
}

// Update updates an account's name
func (r *AccountRepository) Update(account *model.Account) error {
	query := `UPDATE account SET name = ? WHERE account_id = ?`
	result, err := r.db.Exec(query, account.Name, account.AccountID)
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("account not found")
	}

	return nil
}

// Delete deletes an account by ID (cascades to transactions)
func (r *AccountRepository) Delete(accountID int) error {
	query := `DELETE FROM account WHERE account_id = ?`
	result, err := r.db.Exec(query, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("account not found")
	}

	return nil
}

// Count returns the total number of accounts
func (r *AccountRepository) Count() (int, error) {
	query := `SELECT COUNT(*) FROM account`

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count accounts: %w", err)
	}

	return count, nil
}
