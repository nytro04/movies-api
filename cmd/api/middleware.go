package main

import (
	"fmt"
	"net/http"

	"golang.org/x/time/rate"
)

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

func (app *application) rateLimit(next http.Handler) http.Handler {
	// create a new rate limiter with a rate limit of 2 requests per second and a burst limit of 4 requests
	limiter := rate.NewLimiter(2, 4)

	// the function we are returning is a closure that wraps the next http.Handler in the middleware chain
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// call the .Allow() to see if the request is allowed or not, and if it's not,
		// call the rateLimitExceededResponse method to send a 429 Too Many Requests response to the client
		if !limiter.Allow() {
			app.rateLimitExceededResponse(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}
