package main

import (
	"fmt"
	"net/http"
	"time"
)

func (app *application) serve() error {
	// Declare a new HTTP server with some sensible timeout settings, using the same
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.port),
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// log a starting server message
	app.logger.PrintInfo("starting server", map[string]string{
		"addr": srv.Addr,
		"env":  app.config.env,
	})

	// Start the server as normal, returning any error
	return srv.ListenAndServe()
}
