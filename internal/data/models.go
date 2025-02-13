package data

import (
	"database/sql"
	"errors"
)

var (
	ErrRecordNotFound = errors.New("record not found")
	ErrEditConflict   = errors.New("edit conflict")
)

type Models struct {

	// Set the movies field to an interface type containing the methods
	// that both the real and mock movie models must implement(needs to support)
	Movies interface {
		GetAll(title string, genres []string, filters Filters) ([]*Movie, error)
		Insert(movie *Movie) error
		Get(id int64) (*Movie, error)
		Update(movie *Movie) error
		Delete(id int64) error
	}
}

func NewModels(db *sql.DB) Models {
	return Models{
		Movies: MovieModel{DB: db},
	}
}

// helper function which returns models instance containing the modal models only for testing
func NewMockModels() Models {
	return Models{
		Movies: MockMovieModel{},
	}
}
