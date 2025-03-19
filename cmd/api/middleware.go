package main

import (
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/felixge/httpsnoop"
	"github.com/nytro04/greenlight/internal/data"
	"github.com/nytro04/greenlight/internal/validator"
	"github.com/tomasen/realip"
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
		if app.config.limiter.enabled {

			// get client real IP address using the realip package
			ip := realip.FromRequest(r)

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

// requireAuthenticatedUser is a middleware function that checks if the user is not anonymous
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

// requireActivatedUser is a middleware function that checks if the user account is activated
// before calling the next handler in the chain, this will be the requireAuthenticatedUser middleware
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

// requirePermission is a middleware function that checks if the user has the required permission to access a particular route
// the middleware function requires the user to be authenticated and activated(by wrapping the requireActivatedUser around requirePermission i.e app.requireActivatedUser(fn))
//
//	before checking the permissions of the user
func (app *application) requirePermission(code string, next http.HandlerFunc) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		// extract the user from the request context
		user := app.contextGetUser(r)

		// get the slice of permissions for the user
		permissions, err := app.models.Permissions.GetAllForUser(user.ID)
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}

		// check if the user has the required permission
		if !permissions.Include(code) {
			app.notPermittedResponse(w, r)
			return
		}

		// call the next handler in the chain
		next.ServeHTTP(w, r)
	}

	// wrap the handler function in the requireActivatedUser middleware and return it
	return app.requireActivatedUser(fn)
}

func (app *application) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// w.Header().Set("Access-Control-Allow-Origin", "*")

		// add the "Vary: Origin" header to the response. This acts as a hint to any caching middleware
		// that the response will vary depending on the value of the Origin header in the request. This is important
		// because it means that the response will not be cached if the Origin header changes between requests
		w.Header().Add("Vary", "Origin")

		// get the value of the Origin header from the request. This will return an empty string if the header is not present
		origin := r.Header.Get("Origin")

		// if the Origin header is not present, the request is same-origin and we don't need to do anything
		if origin != "" {
			// loop through the list of trusted origins and check if the request Origin header matches any of them
			// if there is a match, set the Access-Control-Allow-Origin header on the response with the value of the Origin header
			// this indicates that the client is allowed to make requests from that origin
			for i := range app.config.cors.trustedOrigins {
				if origin == app.config.cors.trustedOrigins[i] {
					w.Header().Set("Access-Control-Allow-Origin", origin)

					// if the request method is OPTIONS and has an Access-Control-Request-Method header, then we know this is a preflight request
					// in this case, we set the Access-Control-Allow-Methods and Access-Control-Allow-Headers headers on the response
					// and return a 200 OK status code to indicate that the client is allowed to make the request
					if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
						// set the Access-Control-Allow-Methods and Access-Control-Allow-Headers headers on the response
						w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, PUT, PATCH, DELETE")
						w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

						w.WriteHeader(http.StatusOK)
						return
					}
					break
				}
			}
		}
		// call the next handler in the chain
		next.ServeHTTP(w, r)
	})
}

func (app *application) metrics(next http.Handler) http.Handler {
	// declare and initialize the expvar variables when new middleware is created
	totalRequestsReceived := expvar.NewInt("total_requests_received")
	totalResponsesSent := expvar.NewInt("total_responses_sent")
	totalProcessingTimeMicroseconds := expvar.NewInt("total_processing_time_microseconds")
	totalResponsesSentByStatus := expvar.NewMap("total_responses_sent_by_status")

	// following code will be run for every request
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// use the add method to increment the totalRequestsReceived received by 1
		totalRequestsReceived.Add(1)

		// returns the metrics for the request
		metrics := httpsnoop.CaptureMetrics(next, w, r)

		// on the way back up the middleware chain, increment the number of responses sent by 1
		totalResponsesSent.Add(1)

		// increment the total processing time by the duration of the request
		totalProcessingTimeMicroseconds.Add(metrics.Duration.Microseconds())

		// increment the number of responses sent by the status code of the response
		totalResponsesSentByStatus.Add(strconv.Itoa(metrics.Code), 1)
	})
}
