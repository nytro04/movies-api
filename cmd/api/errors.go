package main

import (
	"fmt"
	"net/http"
)

func (app *application) logError(r *http.Request, err error) {
	app.logger.PrintError(err, map[string]string{
		"request_method": r.Method,
		"request_url":    r.URL.String(),
	})
}

// errorResponse method sends a JSON response containing the error message to the client. The status code of the response is passed in the status parameter.
// The message parameter can be a string, or it can be a map with the key "error" containing the error message.
func (app *application) errorResponse(w http.ResponseWriter, r *http.Request, status int, message interface{}) {
	env := envelope{"error": message}

	err := app.writeJSON(w, status, env, nil)
	if err != nil {
		app.logError(r, err)
		w.WriteHeader(500)
	}

}

// serverErrorResponse method sends a 500 Internal Server Error response to the client when an unexpected condition is encountered by the server.
func (app *application) serverErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.logError(r, err)
	message := "the server encountered a problem and could not process your request"
	app.errorResponse(w, r, http.StatusInternalServerError, message)
}

// notFoundResponse method sends a 404 Not Found response to the client when the client sends a request to an endpoint that does not exist.
func (app *application) notFoundResponse(w http.ResponseWriter, r *http.Request) {
	message := "the requested resource could not be found"
	app.errorResponse(w, r, http.StatusNotFound, message)
}

// methodNotAllowedResponse method sends a 405 Method Not Allowed response to the client when the client sends a request to an endpoint that does not support the HTTP method used in the request.
func (app *application) methodNotAllowedResponse(w http.ResponseWriter, r *http.Request) {
	message := fmt.Sprintf("the %s method is not supported for this response", r.Method)
	app.errorResponse(w, r, http.StatusMethodNotAllowed, message)
}

// badRequestResponse method sends a 400 Bad Request response to the client with the error message passed in the err parameter.
// This method is used to send responses when the client sends a request that cannot be processed because the request body is malformed or missing required data.
func (app *application) badRequestResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.errorResponse(w, r, http.StatusBadRequest, err.Error())
}

// failedValidationResponse method sends a 422 Unprocessable Entity response containing the errors map to the client when the request body fails validation checks.
func (app *application) failedValidationResponse(w http.ResponseWriter, r *http.Request, errors map[string]string) {
	app.errorResponse(w, r, http.StatusUnprocessableEntity, errors)
}

// invalidCredentialsResponse method sends a 401 Unauthorized response to the client when the client provides invalid authentication credentials.
func (app *application) invalidCredentialsResponse(w http.ResponseWriter, r *http.Request) {
	message := "invalid authentication credentials"
	app.errorResponse(w, r, http.StatusUnauthorized, message)
}

// editConflictResponse method sends a 409 Conflict response to the client when an edit conflict is detected when trying to update a record in the database that has been modified since it was last fetched.
func (app *application) editConflictResponse(w http.ResponseWriter, r *http.Request) {
	message := "unable to update the record due to an edit conflict, please try again"
	app.errorResponse(w, r, http.StatusConflict, message)
}

// rateLimitExceededResponse method sends a 429 Too Many Requests response to the client when the rate limit is exceeded for a particular route or IP address
func (app *application) rateLimitExceededResponse(w http.ResponseWriter, r *http.Request) {
	message := "rate limit exceeded"
	app.errorResponse(w, r, http.StatusTooManyRequests, message)
}

// invalidAuthenticationTokenResponse method sends a 401 Unauthorized response to the client when the client provides an invalid or missing authentication token.
func (app *application) invalidAuthenticationTokenResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", "Bearer")

	message := "invalid or missing authentication token"
	app.errorResponse(w, r, http.StatusUnauthorized, message)
}

// authenticationRequiredResponse method sends a 401 Unauthorized response to the client when the client tries to access a protected route without providing valid authentication credentials.
func (app *application) authenticationRequiredResponse(w http.ResponseWriter, r *http.Request) {
	message := "you must be authenticated to access this resource"
	app.errorResponse(w, r, http.StatusUnauthorized, message)
}

// inactivateAccountResponse method sends a 403 Forbidden response to the client when the client tries to access a protected route using an account that has not been activated.
func (app *application) inactivateAccountResponse(w http.ResponseWriter, r *http.Request) {
	message := "your account must be activated to access this resource"
	app.errorResponse(w, r, http.StatusForbidden, message)
}
// notPermittedResponse method sends a 403 Forbidden response to the client when the client tries to access a protected route using an account that does not have the necessary permissions.
func (app *application) notPermittedResponse(w http.ResponseWriter, r *http.Request) {
	message := "your use account does not the necessary permissions to access this resource"
	app.errorResponse(w, r, http.StatusForbidden, message)
}
