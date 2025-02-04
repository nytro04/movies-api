package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nytro04/greenlight/internal/data"
)

func (app *application) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title   string   `json:"title"`
		Runtime int32    `json:"runtime"`
		Genres  []string `json:"genres"`
		Year    int32    `json:"year"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	fmt.Fprintf(w, "%+v\n", input)

}

func (app *application) showMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	movie := data.Movie{
		ID:        id,
		CreatedAt: time.Now(),
		Title:     "Casablanca",
		Runtime:   120,
		Genres:    []string{"drama", "mystery", "thriller"},
		Version:   1,
	}

	// err = app.writeJSON(w, http.StatusOK, envelope{"movie": movie} , nil) //using envelope type
	err = app.writeJSON(w, http.StatusOK, movie, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
