package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

// Insert method to create a new movie record
func (m MovieModel) Insert(movie *Movie) error {
	query := `
		INSERT INTO movies (title, year, runtime, genres)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, version`

	// Create a slice containing the movie
	args := []interface{}{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres)}

	// create a new context with a 3-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.ID, &movie.CreatedAt, &movie.Version)
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

	// Create a context with a 3-second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Use the QueryRow() method to execute the query and scan the returned row into the movie struct.
	err := m.DB.QueryRowContext(ctx, query, id).Scan(
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

func (m MovieModel) GetAll(title string, genres []string, filters Filters) ([]*Movie, Metadata, error) {
	// The query to retrieve all movies records. The query uses a WHERE clause to filter the results based on the title and genres.
	// title will be matched using a case-insensitive search or empty string, and genres will be matched using the @> operator to check if the genres column contains all of the genres in the slice or pass an empty array.
	// full text search is used to search the title column. to_tsvector('simple', title), splits the title into lexemes eg. "the matrix" -> 'the' 'matrix', we use 'simple' configuration to turn it into lowercase and remove punctuation.
	// the plainto_tsquery turns title into a formatted query that POSTGRES full text search can understand.
	// sort the results based on the sort column and direction provided in the filters struct(interpolation is used to insert the column and direction into the query).
	// add a secondary sort on the movie ID to ensure that the results are returned in a consistent order.
	// add a window function(count(*) OVER()) to count the total number of records that match the query, and return this as a column in the result set.
	query := fmt.Sprintf(
		`SELECT count(*) OVER(), id, created_at, title, year, runtime, genres, version
	   FROM movies
	   WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '')
	   AND (genres @> $2 OR $2 = '{}')
	   ORDER BY %s %s, id ASC
	   LIMIT $3 OFFSET $4`, filters.sortColumn(), filters.sortDirection())

	// Create a new context with a 3-second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// values of sql placeholders parameters in a slice
	args := []interface{}{title, pq.Array(genres), filters.limit(), filters.offset()}

	// Execute the query passing in the title and genres as the placeholders. If an error is returned, return it to the calling function.
	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}

	// defer closing the rows
	defer rows.Close()

	// declare totalRecords variable to hold the total number of records that match the query
	totalRecords := 0
	// iniitialize an empty slice to hold the movie records
	movies := []*Movie{}

	// Iterate through the rows returned by the query.
	for rows.Next() {
		// Create a new Movie struct to hold the data for each movie record.
		var movie Movie

		// Scan the values from the row into the Movie struct.
		err := rows.Scan(
			&totalRecords,
			&movie.ID,
			&movie.CreatedAt,
			&movie.Title,
			&movie.Year,
			&movie.Runtime,
			pq.Array(&movie.Genres),
			&movie.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}
		// Append the Movie struct to the slice.
		movies = append(movies, &movie)
	}

	// If an error was encountered while iterating through the rows, return it to the calling function.
	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}
	// generate the metadata struct, passing in the total number of records, the current page, and the page size.
	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return movies, metadata, nil
}

// Update method to update the movie record
func (m MovieModel) Update(movie *Movie) error {
	// query for updating the movie record
	query := `
	UPDATE movies
	SET title = $1, year = $2, runtime = $3, genres = $4, version = version + 1
	WHERE ID = $5 AND version = $6
	RETURNING version`

	// Create a slice containing the movie genres
	args := []interface{}{
		movie.Title,
		movie.Year,
		movie.Runtime,
		pq.Array(movie.Genres),
		movie.ID,
		movie.Version,
	}

	// Create a new context with a 3-second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Execute the query. If no matching row is found, we know that the movie version has changed
	// or the movie has been deleted, so we return ErrEditConflict.
	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}
	return nil
}

// Delete method to delete the movie record
func (m MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	// delete query
	query := `
	DELETE FROM movies
	WHERE id = $1`

	// Create a new context with a 3-second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Execute the query, passing the id as the value for the placeholder parameter.
	result, err := m.DB.ExecContext(ctx, query, id)
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

func (m MockMovieModel) GetAll(title string, genres []string, filters Filters) ([]*Movie, Metadata, error) {
	return nil, Metadata{}, nil
}

func (m MockMovieModel) Update(movie *Movie) error {
	return nil
}

func (m MockMovieModel) Delete(id int64) error {
	return nil
}
