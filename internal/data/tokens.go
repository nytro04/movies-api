package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"time"

	"github.com/nytro04/greenlight/internal/validator"
)

// Define constants for the token scope values
const (
	ScopeActivation     = "activation"
	ScopeAuthentication = "authentication"
)

// Define a Token struct to hold the data for a single token. This will be used to read and write token data to and from the database
// The Plaintext field will store the plaintext version of the token, which will be sent to the user in the activation email.
// The Hash field will store the hashed version of the token, which will be stored in the database.
type Token struct {
	Plaintext string    `json:"token"`
	Hash      []byte    `json:"-"`
	UserID    int64     `json:"-"`      // the ID of the user the token belongs to
	Expiry    time.Time `json:"expiry"` // the expiry time of the token
	Scope     string    `json:"-"`      // the scope of the token
}

// generateToken generates a new token for the user with the provided user ID, expiry time, and scope.
// The method generates a random 16-byte plaintext token, encodes it to a base32-encoded string, and stores it in the Plaintext field of the token.
// It then generates the SHA-256 hash of the plaintext token and stores it in the Hash field of the token.
// The method returns the token and a nil error if the token is generated successfully.
// If an error occurs when generating the random bytes, the method returns nil and the error.
func generateToken(userID int64, ttl time.Duration, scope string) (*Token, error) {
	// Generate a new token with the provided user ID, expiry time, and scope.
	// Notice we add the provided ttl(time to live) to the current time to get the expiry time of the token.
	token := &Token{
		UserID: userID,
		Expiry: time.Now().Add(ttl),
		Scope:  scope,
	}

	// Initialize a zero-valued byte slice with a length of 16 bytes.
	randomBytes := make([]byte, 16)

	// Use the Read method from the crypto/rand package to fill the byte slice with random bytes.
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil, err
	}

	// Encode the random bytes to a base32-encoded string and store it in the Plaintext field of the token.
	// We use the WithPadding(base32.NoPadding) method to remove padding from the encoded string.
	token.Plaintext = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes)

	// Generate the SHA-256 hash of the plaintext token and store it in the Hash field of the token, which will be stored in the database.
	// Note the use of the [:] operator to convert the 32-byte array to a byte slice. This is required because the Hash field is a byte slice.
	hash := sha256.Sum256([]byte(token.Plaintext))
	token.Hash = hash[:]

	return token, nil
}

// Check that the plaintext token is provided and is 26 bytes long.
func ValidateTokenPlaintext(v *validator.Validator, tokenPlaintext string) {
	v.Check(tokenPlaintext != "", "token", "must be provided")
	v.Check(len(tokenPlaintext) == 26, "token", "must be 26 bytes long")
}

// Define the TokenModel type
type TokenModel struct {
	DB *sql.DB
}

// The New method is a shortcut for generating a new token struct and inserting it into the tokens table.
func (m TokenModel) New(userID int64, ttl time.Duration, scope string) (*Token, error) {
	token, err := generateToken(userID, ttl, scope)
	if err != nil {
		return nil, err
	}

	err = m.Insert(token)
	return token, err
}

// Insert method to create a new token record in the tokens table
func (m TokenModel) Insert(token *Token) error {
	query := `
	INSERT INTO tokens (hash, user_id, expiry, scope)
	VALUES ($1, $2, $3, $4)
	`
	// Create a slice containing the token struct fields to be inserted into the database.
	args := []interface{}{token.Hash, token.UserID, token.Expiry, token.Scope}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, args...)
	return err
}

// DeleteAllForUser method to delete all tokens for a specific user and scope
func (m TokenModel) DeleteAllForUser(scope string, userID int64) error {
	query := `
	DELETE FROM tokens
	WHERE scope = $1 AND user_id = $2
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := m.DB.ExecContext(ctx, query, scope, userID)
	return err
}

// MockTokenModel type to help with testing
type MockTokenModel struct{}

func (m MockTokenModel) New(userID int64, ttl time.Duration, scope string) (*Token, error) {
	return nil, nil
}

func (m MockTokenModel) Insert(token *Token) error {
	return nil
}

func (m MockTokenModel) DeleteAllForUser(scope string, userID int64) error {
	return nil
}
