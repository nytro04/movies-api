package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/nytro04/greenlight/internal/data"
	"github.com/nytro04/greenlight/internal/validator"
)

func (app *application) createActivationTokenHandler(w http.ResponseWriter, r *http.Request) {
	// parse and validate user email
	var input struct {
		Email string `json:"email"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()

	if data.ValidateEmail(v, input.Email); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	// try to retrieve the corresponding user record for the email address. if it cant
	// be found, return am error message to the client
	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			v.AddError("email", "no user found with this email address")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	// return an error if the user account is already activated
	if user.Activated {
		v.AddError("email", "user account is already activated")
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	// create a new activation token for the user
	token, err := app.models.Tokens.New(user.ID, 3*24*time.Hour, data.ScopeActivation)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// we send the email to the user in the background to avoid blocking the request
	app.background(func() {
		data := map[string]interface{}{
			"activationToken": token.Plaintext,
		}

		// we send the email to email address of the user and not the one provided in the request
		// this is to avoid leaking the email address of the user to the client in case of an error.
		err = app.mailer.Send(user.Email, "token_activation.go.tmpl", data)
		if err != nil {
			app.logger.PrintError(err, nil)
		}
	})

	// send a 202 Accepted status code and a JSON response containing a success message
	env := envelope{"message": "an email will be sent to you containing the activation instructions"}

	err = app.writeJSON(w, http.StatusAccepted, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
