package model

import "time"

// Account represents a user-defined bank account or credit card
type Account struct {
	AccountID int       `json:"account_id" db:"account_id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// NewAccount creates a new Account with the given name
func NewAccount(name string) *Account {
	return &Account{
		Name:      name,
		CreatedAt: time.Now(),
	}
}
