package data

import (
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/nytro04/greenlight/internal/validator"
)

type Movie struct {
	ID        int64     `json:"id"`        // Unique integer ID for the movie
	CreatedAt time.Time `json:"createdAt"` // Timestamp for when the movie is added to our database
	Title     string    `json:"title"`     // Movie title
	Year      int32     `json:"year"`      // Movie release year
	Runtime   Runtime   `json:"runtime"`   // Movie runtime (in minutes)
	Genres    []string  `json:"genres"`    // Slice of genres for the movie (romance, comedy, etc.)
	Version   int32     `json:"version"`   // The version number starts at 1 and will be incremented each // time the movie information is updated
}

func ValidateMovie(v *validator.Validator, movie *Movie) {
	v.Check(movie.Title != "", "title", "must be provided")
	v.Check(len(movie.Title) < 500, "title", "must not be more than 500 bytes long")

	v.Check(movie.Year != 0, "year", "must be provided")
	v.Check(movie.Year >= 1888, "year", "must be greater than 1888")
	v.Check(movie.Year <= int32(time.Now().Year()), "year", "must not be in the future")

	v.Check(movie.Runtime != 0, "runtime", "must be provided")
	v.Check(movie.Runtime > 0, "runtime", "must be a positive integer")

	v.Check(movie.Genres != nil, "genres", "must be provided")
	v.Check(len(movie.Genres) >= 1, "genres", "must contain at least 1 genre")
	v.Check(len(movie.Genres) <= 5, "genres", "must not contain more than 5 genres")
	v.Check(validator.Unique(movie.Genres), "genres", "must not contain duplicate values")
}

type MovieModel struct {
	DB *sql.DB
}

func (m MovieModel) Insert(movie *Movie) error {
	query := `
		INSERT INTO movies (title, year, runtime, genres)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, version`

	// Create a slice containing the movie genres
	args := []interface{}{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres)}

	return m.DB.QueryRow(query, args...).Scan(&movie.ID, &movie.CreatedAt, &movie.Version)
}

func (m MovieModel) Get(id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
	SELECT id, created_at, title, year, runtime, genres, version
	FROM movies
	WHERE id = $1`

	var movie Movie

	// Use the QueryRow() method to execute the query and scan the returned row into the movie struct.
	err := m.DB.QueryRow(query, id).Scan(
		&movie.ID,
		&movie.CreatedAt,
		&movie.Title,
		&movie.Year,
		&movie.Runtime,
		pq.Array(&movie.Genres),
		&movie.Version,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &movie, nil

}

func (m MovieModel) Update(movie *Movie) error {
	// query for updating the movie record
	query := `
	UPDATE movies
	SET title = $1, year = $2, runtime = $3, genres = $4, version = version + 1
	WHERE ID = $5
	RETURNING version`

	// Create a slice containing the movie genres
	args := []interface{}{
		movie.Title,
		movie.Year,
		movie.Runtime,
		pq.Array(movie.Genres),
		movie.ID,
	}

	// Execute the query and scan the returned version number into the movie struct.
	return m.DB.QueryRow(query, args...).Scan(&movie.Version)
}

func (m MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	// delete query
	query := `
	DELETE FROM movies
	WHERE id = $1`

	// Execute the query, passing the id as the value for the placeholder parameter.
	result, err := m.DB.Exec(query, id)
	if err != nil {
		return err
	}

	// Check how many rows were affected by the query.
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	// If no rows were affected, we know that the movie with that ID doesn't exist in the database.
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

// Mock data for testing
type MockMovieModel struct{}

func (m MockMovieModel) Insert(movie *Movie) error {
	return nil
}

func (m MockMovieModel) Get(id int64) (*Movie, error) {
	return nil, nil
}

func (m MockMovieModel) Update(movie *Movie) error {
	return nil
}

func (m MockMovieModel) Delete(id int64) error {
	return nil
}
