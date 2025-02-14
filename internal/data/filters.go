package data

import (
	"strings"

	"github.com/nytro04/greenlight/internal/validator"
)

type Filters struct {
	Page         int
	PageSize     int
	Sort         string
	SortSafeList []string
}

// check that the client provided sort field matches one of the safe values in the SortSafeList
// if it does, return the field name without the "-" prefix
// if it doesn't, panic with a message indicating that the client provided an unsafe value
func (f Filters) sortColumn() string {
	for _, safeValue := range f.SortSafeList {
		if f.Sort == safeValue {
			return strings.TrimPrefix(f.Sort, "-")
		}
	}
	panic("unsafe sort parameter: " + f.Sort)
}

// Return the sort direction (ASC or DESC) based on the prefix of the Sort field
func (f Filters) sortDirection() string {
	if strings.HasPrefix(f.Sort, "-") {
		return "DESC"
	}
	return "ASC"
}

func ValidateFilters(v *validator.Validator, f Filters) {
	// check that the page and page_size parameters contain sensible values
	v.Check(f.Page > 0, "page", "must be greater than zero")
	v.Check(f.Page <= 10_000_000, "page", "must be a maximum of 10 million")
	v.Check(f.PageSize > 0, "page_size", "must be greater than zero")
	v.Check(f.PageSize <= 100, "page_size", "must be a maximum of 100")

	// check that the sort parameter matches a value in the safe list
	v.Check(validator.In(f.Sort, f.SortSafeList...), "sort", "invalid sort value")
}
