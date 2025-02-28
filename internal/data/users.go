package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"github.com/nytro04/greenlight/internal/validator"
	"golang.org/x/crypto/bcrypt"
)

// Define a custom ErrDuplicateEmail error. This will be used to indicate that a user with the specified email address already exists in the database
var (
	EmailDuplicateKeyConstraint = `pq: duplicate key value violates unique constraint "users_email_key"`
	ErrDuplicateEmail           = errors.New("duplicate email")
)

// Define a UserModel struct type which wraps the connection pool .This struct will be used to read and write user data to and from the database
type UserModel struct {
	DB *sql.DB
}

// Define a User struct to hold the data for a single user. This will be used to read and write user data to and from the database
type User struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  password  `json:"-"` // use the "-" to tell the json package to ignore this field
	Activated bool      `json:"activated"`
	Version   int       `json:"-"` // use the "-" to tell the json package to ignore this field
}

// create a custom type to represent a password. This will be used to store the plaintext password and the hashed version of the password
// the plaintext field is a pointer to a string, which means that it can be nil. This will allow us to differentiate between a password that has not been set and a password that has been set to an empty string (i.e. "")
// the hash field is a byte slice that will store the hashed version of the password
type password struct {
	plaintext *string
	hash      []byte
}

// generate the bcrypt hash of a plaintext password and store both the plaintext and hashed versions of the password in the password struct
func (p *password) HashPassword(plaintextPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), 12) // use a cost of 12 to generate the bcrypt hash
	if err != nil {
		return err
	}
	p.plaintext = &plaintextPassword
	p.hash = hash

	return nil
}

// check if a plaintext password matches the hashed password stored in the password struct. This method returns true if the passwords match, or false if they do not
func (p *password) Matches(plaintextPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword(p.hash, []byte(plaintextPassword))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err
		}
	}

	return true, nil
}

// validate the email address using the validator package. The email address must be provided and must be a valid email address
func ValidateEmail(v *validator.Validator, email string) {
	v.Check(email != "", "email", "must be provided")
	v.Check(validator.Matches(email, validator.EmailRX), "email", "must be a valid email address")
}

// validate the plaintext password using the validator package. The password must be at least 8 bytes long and no more than 72 bytes long
func ValidatePasswordPlaintext(v *validator.Validator, password string) {
	v.Check(password != "", "password", "must be provided")
	v.Check(len(password) >= 8, "password", "must be at least 8 bytes long")
	v.Check(len(password) <= 72, "password", "must not be more than 72 bytes long")

	// TODO: Add additional checks for password strength (e.g. requiring a mix of uppercase and lowercase letters, numbers, and symbols)
}

// validate the user data using the validator package. This function will validate the name field is not empty and not more than 500 bytes long, and then
//
//	call the ValidateEmail and ValidatePasswordPlaintext helper functions to validate the email address and password
func ValidateUser(v *validator.Validator, user *User) {
	v.Check((user.Name != ""), "name", "must be provided")
	v.Check((len(user.Name) <= 500), "name", "must not be more than 500 bytes long")

	// validate the email address using the ValidateEmail helper
	ValidateEmail(v, user.Email)

	// if the plaintext password is not nil, validate it using the ValidatePasswordPlaintext helper
	if user.Password.plaintext != nil {
		ValidatePasswordPlaintext(v, *user.Password.plaintext)
	}

	// if the password is ever nil, this will be due to a logic error in our codebase(probably we forgot to set a password for the user)
	// it's a useful sanity check to include here, but it's not a problem with the data provided by the client. so rather than using the clientError helper to return a 400 Bad Request response, we'll use the panic function to trigger a panic
	// So we'll use the internalError helper to log a message and return a 500 Internal Server Error response

	// look into making this a custom error type instead of using panic
	if user.Password.hash == nil {
		// set error message
		// v.AddError("password", "not not provided")
		panic("missing password for user")
	}
}

// Insert a new user record in the database for the user. Note that the id, created_at, and version fields are all automatically generated by the database.
// so we use the RETURNING clause to read them back into the user struct after the insert, and update the fields accordingly
func (m UserModel) Insert(user *User) error {
	query := `
		INSERT INTO users (name, email, password_hash, activated)
		VALUES($1, $2, $3, $4)
		RETURNING id, created_at, version
	`

	args := []interface{}{user.Name, user.Email, user.Password.hash, user.Activated}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// use QueryRowContext to execute the query and scan the returned id, created_at, and version values into the user struct
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&user.ID, &user.CreatedAt, &user.Version)
	if err != nil {
		switch {
		// if the table already contains a record with this email address, then when we try to perform the insert, there will a violation of the UNIQUE "users_email_key"
		// constraint, we can check for this specific error message and return our custom ErrDuplicateEmail error
		case err.Error() == EmailDuplicateKeyConstraint:
			return ErrDuplicateEmail
		default:
			return err
		}
	}

	return nil
}

// Retrieve the User details from the database based on the user's email address.
// Because we have a UNIQUE constraint on the email column, , this SQL query will only return
// one record (or none at all, in which case we return ErrRecordNotFound)
func (m UserModel) GetByEmail(email string) (*User, error) {
	query := `
		SELECT id, created_at, name, email, password_hash, activated, version
		FROM users
		WHERE email = $1
	`

	var user User

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.Name,
		&user.Email,
		&user.Password.hash,
		&user.Activated,
		&user.Version,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &user, nil
}

// Update the details for a specific user. Notice that we check against the version field to help prevent any race conditions during the request cycle.
// we also check for a violation of the UNIQUE "users_email_key" constraint and return our custom ErrDuplicateEmail error if this occurs
func (m UserModel) Update(user *User) error {
	query := `
		UPDATE users
		SET name = $1, email = $2, password_hash = $3, activated = $4, version = version + 1
		WHERE id = $5 AND version = $6
		RETURNING version
	`

	args := []interface{}{
		user.Name,
		user.Email,
		user.Password.hash,
		user.Activated,
		user.ID,
		user.Version,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&user.Version)
	if err != nil {
		switch {
		// if the table already contains a record with this email address, then when we try to perform the insert, there will a violation of the UNIQUE "users_email_key"
		// constraint, we can check for this specific error message and return our custom ErrDuplicateEmail error
		case err.Error() == EmailDuplicateKeyConstraint:
			return ErrDuplicateEmail
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

// This method will retrieve the user details based on the token hash, scope,
// It will return the user details if a matching record is found, or an error if no matching record is found
func (m UserModel) GetTokenUser(tokenScope, tokenPlaintext string) (*User, error) {
	// hash the plaintext token using the SHA-256 algorithm, returning a 32-byte array
	tokenHash := sha256.Sum256([]byte(tokenPlaintext))

	// query to retrieve the user details based on the token hash, scope and expiry time
	query := `
		SELECT users.id, users.created_at, users.name, users.email, users.password_hash, users.activated, users.version
		FROM users
		INNER JOIN tokens
		ON users.id = tokens.user_id
		WHERE tokens.hash = $1
		AND tokens.scope = $2
		AND tokens.expiry > $3`

	// create a slice containing the query arguments. The token hash is converted to a byte slice using the [:] operator
	// because the pq driver expects a byte slice. we pass the current time against the token expiry time to check if the token is still valid
	args := []interface{}{tokenHash[:], tokenScope, time.Now()}

	var user User

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// execute the query and scan the returned values into the user struct, returning ErrRecordNotFound if no matching record is found
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.Name,
		&user.Email,
		&user.Password.hash,
		&user.Activated,
		&user.Version,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	// return matching user
	return &user, nil
}

// Mock data for testing
type MockUserModel struct{}

func (m MockUserModel) Insert(user *User) error {
	return nil
}

func (m MockUserModel) GetByEmail(email string) (*User, error) {
	return nil, nil
}

func (m MockUserModel) Update(user *User) error {
	return nil
}

func (m MockUserModel) GetTokenUser(tokenScope, tokenPlaintext string) (*User, error) {
	return nil, nil
}
