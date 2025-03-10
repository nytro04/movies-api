package main

import (
	"context"
	"net/http"

	"github.com/nytro04/greenlight/internal/data"
)

// contextKey is a custom type that we will use as the key for storing request values in the request context.
// This is a neat trick to prevent collisions with other packages that might be using the same key name.
type contextKey string

// Convert the string "user " to a contextKey type and store it in a constant named userContextKey.
// We will use this constant as the key when storing and retrieving the User info from the request context.
const userContextKey = contextKey("user")

// Define a new contextSetUser helper. This returns a new copy of the request with the specified User struct added to the context.
// note that we use our custom contextKey type as the key. This helps to prevent collisions with other data stored in the context.
func (app *application) contextSetUser(r *http.Request, user *data.User) *http.Request {
	ctx := context.WithValue(r.Context(), userContextKey, user)
	return r.WithContext(ctx)
}

// contextGetUser retrieves the User struct from the request context. This will only be called after the user has been authenticated and authorized.
// If the user value doesn't exist in the context for some reason, we log a message using the app.logger.PrintFatal() method and then call the panic() function to stop the application.
// This should never happen, but it's better to be safe than sorry.
func (app *application) contextGetUser(r *http.Request) *data.User {
	user, ok := r.Context().Value(userContextKey).(*data.User)
	if !ok {
		// find a way to log the error
		panic("missing user value in request context")
	}
	return user
}
