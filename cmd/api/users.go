package main

import (
	"errors"
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

	// Use the background helper to execute an anonymous function that sends a welcome email to the user in the background
	app.background(func() {
		err = app.mailer.Send(user.Email, "user_welcome.go.tmpl", user)
		if err != nil {
			app.logger.PrintError(err, nil)
			return
		}
	})

	err = app.writeJSON(w, http.StatusAccepted, envelope{"user": user}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
