package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nytro04/greenlight/internal/data"
	"github.com/nytro04/greenlight/internal/validator"
	"golang.org/x/time/rate"
)

// recoverPanic is a middleware function that recovers from panics in the application and returns a 500 Internal Server Error response to the client.
func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// defer a function to recover from a panic.
		// if a panic occurs, set the Connection: close header on the response
		defer func() {
			// the built-in recover function checks if there has been a panic or not
			if err := recover(); err != nil {
				// if there was a panic, set a "Connection: close" header on the response.
				// this acts as a trigger to force Go's HTTP server to close the current connection after a response has been sent
				w.Header().Set("Connection", "close")

				// call the serverErrorResponse method to send a 500 Internal Server Error response to the client
				app.serverErrorResponse(w, r, fmt.Errorf("%s", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// rateLimit is a middleware function that rate-limits the number of requests that clients can make to specific endpoints.
func (app *application) rateLimit(next http.Handler) http.Handler {

	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	// Declare a mutex and a map to hold the clients IP addresses and their associated rate limiter
	var (
		mu      sync.Mutex
		clients = make(map[string]*client)
	)

	// Launch a background goroutine that removes old entries from the clients map once every minute
	go func() {
		for {
			time.Sleep(time.Minute)
			// Lock the mutex to prevent any other goroutines from accessing the map while we're deleting the old entries
			mu.Lock()

			// Loop through all clients. If they haven't been seen within the last 3 minutes, delete the corresponding entry from the map
			for ip, client := range clients {
				if time.Since(client.lastSeen) > 3*time.Minute {
					delete(clients, ip)
				}
			}
			// Unlock the mutex when the cleanup is complete. This will allow other goroutines to access the map again
			mu.Unlock()
		}
	}()

	// the function we are returning is a closure that wraps the next http.Handler in the middleware chain
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// extract the client's IP address from the request
		if app.config.limiter.enabled {

			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				app.serverErrorResponse(w, r, err)
				return
			}

			// Lock the mutex to protect the map from concurrent access
			mu.Lock()

			// check if the IP address is already in the map. if it's not, create a new rate limiter and add the IP address and limiter to the map
			if _, found := clients[ip]; !found {
				// create and add a new client struct to the map if it doesn't already exist
				clients[ip] = &client{limiter: rate.NewLimiter(rate.Limit(app.config.limiter.rps), app.config.limiter.burst)} // 2 requests per second, with a maximum of 4 requests in a burst
			}

			// Update the last seen time for the client
			clients[ip].lastSeen = time.Now()

			// call the .Allow() method on the current rate limiter. if the request isn't allowed, unlock the mutex and
			// call the rateLimitExceededResponse method to send a 429 Too Many Requests response to the client
			if !clients[ip].limiter.Allow() {
				mu.Unlock()
				app.rateLimitExceededResponse(w, r)
				return
			}

			// unlock the mutex and call the next handler in the chain
			mu.Unlock()
		}

		next.ServeHTTP(w, r)
	})
}

// authenticate is a middleware function that checks whether a request is authorized by looking for a valid authentication token in the Authorization header.
// If the request is authorized, the user details are added to the request context. If the request is not authorized, a 401 Unauthorized response is sent to the client.
func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// add the "Vary: Authorization" header to the response. This acts as a hint to any caching middleware
		// that the response will vary depending on the value of the Authorization header in the request.
		w.Header().Add("Vary", "Authorization")

		// retrieve the value of the Authorization header from the request. This will return an empty string "" if the header is not present
		authorizationHeader := r.Header.Get("Authorization")

		// if there is no Authorization header, call the contextSetUser() method to add the AnonymousUser to the request context
		// and then call the next handler in the chain and return without executing any of the code below
		if authorizationHeader == "" {
			r = app.contextSetUser(r, data.AnonymousUser)
			next.ServeHTTP(w, r)
			return
		}

		// we expect the value of the Authorization header to be in the format "Bearer <token>", we try split this into two parts"
		// if the header is not in the expected format, we return a 401 Unauthorized response.

		headerParts := strings.Split(authorizationHeader, " ")
		if len(headerParts) != 2 || headerParts[0] != "Bearer" {
			app.invalidAuthenticationTokenResponse(w, r)
			return
		}

		// extract the actual token from the header parts
		token := headerParts[1]

		// validate the token to make sure it is in a sensible format
		// if the token is invalid, return a 401 Unauthorized response
		v := validator.New()
		if data.ValidateTokenPlaintext(v, token); !v.Valid() {
			app.invalidAuthenticationTokenResponse(w, r)
			return
		}

		// retrieve the details of the user associated with the authentication token, and handle any errors
		user, err := app.models.Users.GetTokenUser(data.ScopeAuthentication, token)
		if err != nil {
			switch {
			case errors.Is(err, data.ErrRecordNotFound):
				app.invalidAuthenticationTokenResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}

		// call the contextSetUser() method to add the user information to the request context
		r = app.contextSetUser(r, user)

		// call the next handler in the chain
		next.ServeHTTP(w, r)

	})
}

// requireActivatedUser is a middleware function that checks if the user is not anonymous
func (app *application) requireAuthenticatedUser(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// extract the user information from the request context
		user := app.contextGetUser(r)

		// if the user is anonymous, call the authenticationRequiredResponse method and return
		if user.IsAnonymous() {
			app.authenticationRequiredResponse(w, r)
			return
		}

		// call the next handler in the chain
		next.ServeHTTP(w, r)

	})
}

func (app *application) requireActivatedUser(next http.HandlerFunc) http.HandlerFunc {
	// rather than returning an http.HandlerFunc, we assign the handler function to a variable
	fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)

		// if the user account is not activated, call the inactivateAccountResponse method and return
		if !user.Activated {
			app.inactivateAccountResponse(w, r)
			return
		}

		// call the next handler in the chain
		next.ServeHTTP(w, r)
	})

	// wrap the handler function in the requireAuthenticatedUser middleware and return it
	return app.requireAuthenticatedUser(fn)
}
