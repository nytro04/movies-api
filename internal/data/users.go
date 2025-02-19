package data

import (
	"errors"
	"time"

	"github.com/nytro04/greenlight/internal/validator"
	"golang.org/x/crypto/bcrypt"
)

// Define a custom ErrDuplicateEmail error. This will be used to indicate that a user with the specified email address already exists in the database
var (
	ErrDuplicateEmail = errors.New("duplicate email")
)

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

// calculate the bcrypt hash of a plaintext password and store both the plaintext and hashed versions of the password in the password struct
func (p *password) HashPassword(plaintextPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), 12)
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

func ValidateEmail(v *validator.Validator, email string) {
	v.Check(email != "", "email", "must be provided")
	v.Check(validator.Matches(email, validator.EmailRX), "email", "must be a valid email address")
}

func ValidatePasswordPlaintext(v *validator.Validator, password string) {
	v.Check(password != "", "password", "must be provided")
	v.Check(len(password) >= 8, "password", "must be at least 8 bytes long")
	v.Check(len(password) <= 72, "password", "must not be more than 72 bytes long")
}

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
	if user.Password.hash != nil {
		// set error message
		v.AddError("password", "not not provided")
		// panic("password hash should be nil")
	}
}
