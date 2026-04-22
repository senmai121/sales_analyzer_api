package models

import "time"

// User represents a row in the users table.
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Username  string    `json:"username,omitempty"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// RegisterRequest is the body for POST /api/auth/register.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
}

// LoginRequest is the body for POST /api/auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse is returned by both register and login.
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}
