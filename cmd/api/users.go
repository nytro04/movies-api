package main

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/nytro04/greenlight/internal/data"
	"github.com/nytro04/greenlight/internal/validator"
)

// create a new handler for the /v1/users/register endpoint that expects a POST request
func (app *application) registerUserHandler(w http.ResponseWriter, r *http.Request) {
	// create an anonymous struct to hold the expected request body
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// parse the request body into the anonymous struct
	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// create a new User struct containing the data from the request body
	user := &data.User{
		Name:      input.Name,
		Email:     input.Email,
		Activated: false,
	}

	// Use the HashPassword method to generate and store the hashed and plaintext versions of the password
	err = user.Password.HashPassword(input.Password)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	v := validator.New()

	// validate the user struct
	if data.ValidateUser(v, user); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = app.models.Users.Insert(user)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrDuplicateEmail):
			v.AddError("email", "a user with this email address already exists")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	// launch a goroutine which runs an anonymous function that sends the welcome email to the user
	go func() {

		// Run a deferred function which uses recover() to catch any runtime panics and log the error using the application logger
		// instead of terminating the application
		defer func ()  {
			if err := recover(); err != nil {
				app.logger.PrintError(fmt.Errorf("%s", err), nil)
			}
		}()

		// call the Send method on our Mailer, passing in the user's email address, the name of the email template file, and the user struct
		// the Send method will render the email template, and then send the email to the user using the SMTP server settings
		err = app.mailer.Send(user.Email, "user_welcome.go.tmpl", user)
		if err != nil {
			app.logger.PrintError(err, nil)
			return
		}
	}()

	err = app.writeJSON(w, http.StatusAccepted, envelope{"user": user}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
