package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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

	// Declare a shutdownError channel to receive any errors returned by the graceful shutdown process
	shutdownError := make(chan error)

	go func() {
		// create a quit channel which carries os.Signal values
		quit := make(chan os.Signal, 1)

		// use signal.Notify() to listen for incoming signals and relay them to the quit channel
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

		// read the signal from the quit channel. this code will block until a signal is received.
		s := <-quit

		// log a message to say that the signal has been caught, along with the signal type(name) as a string
		app.logger.PrintInfo("shutting down server", map[string]string{
			"signal": s.String(),
		})

		// create a context with a 5-second timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// call the Shutdown() method on our server, passing in the context. this will gracefully shutdown the server without interrupting any active connections
		// if the context deadline is exceeded, the Shutdown() method will return a context.Canceled error (or other context-related errors) which will be sent to the shutdownError channel
		// if the server is able to shut down before the context deadline is exceeded, any error returned by the Shutdown() method will be nil (i.e. the server shut down successfully)
		// in either case, the error returned by Shutdown() is sent to the shutdownError channel for logging and debugging purposes
		shutdownError <- srv.Shutdown(ctx)
	}()

	// log a starting server message
	app.logger.PrintInfo("starting server", map[string]string{
		"addr": srv.Addr,
		"env":  app.config.env,
	})

	// calling shutdown() on our server will cause the Serve() method to immediately return an http.ErrServerClosed error.
	// so if we see this error, it's actually a good thing and an indication that the graceful shutdown has started.
	// so we check specifically for this, only returning the error if it's not http.ErrServerClosed
	err := srv.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	// otherwise we wait to receive the return value from the shutdown() on the shutdownError channel.
	// if return value is an error, we know that there was a problem with the graceful shutdown process, so we return the error
	err = <-shutdownError
	if err != nil {
		return err
	}

	// at this point, we know that the graceful shutdown completed successfully, so we log a message to say so
	app.logger.PrintInfo("stopped server", map[string]string{
		"addr": srv.Addr,
	})

	return nil
}
