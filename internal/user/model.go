package user

import "time"

type Role string

const (
	RoleRenter Role = "renter"
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
)

type PasswordAlgo string

const (
	PasswordAlgoBcrypt PasswordAlgo = "bcrypt"
	PasswordAlgoScrypt PasswordAlgo = "scrypt"
)

type User struct {
	ID           int64        `db:"id" json:"id"`
	Email        string       `db:"email" json:"email"`
	PasswordHash string       `db:"password_hash" json:"-"`
	PasswordAlgo PasswordAlgo `db:"password_algo" json:"-"`
	DisplayName  string       `db:"display_name" json:"display_name"`
	Role         Role         `db:"role" json:"role"`
	CreatedAt    time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt    time.Time    `db:"updated_at" json:"updated_at"`
}
