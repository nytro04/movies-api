package main

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

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
