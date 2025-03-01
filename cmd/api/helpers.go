package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/nytro04/greenlight/internal/validator"
)

// helper method to extract the id parameter from the request context and convert it to an integer.
// If the id parameter cannot be parsed as an integer, or is less than 1, the method returns an error.
func (app *application) readIDParam(r *http.Request) (int64, error) {

	params := httprouter.ParamsFromContext(r.Context())

	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id parameter")
	}

	return id, nil
}

// define envelope type
type envelope map[string]interface{}

// writeJSON helper writes the provided data to the response writer, with the provided HTTP status code. It also sets the Content-Type header to application/json.
// If we want to include additional headers in the response, we can add them to the headers parameter which is a map of string slices.
// The keys in the map are the header names, and the values are the header values. The method returns an error if the JSON data cannot be encoded, or if writing to the response writer fails.
func (app *application) writeJSON(w http.ResponseWriter, status int, data interface{}, headers http.Header) error {
	// encode the data to JSON, if there is an error, return it
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// append a newline to make the response easier to read
	js = append(js, '\n')

	// add any additional headers to the response writer header map if they are provided
	for key, value := range headers {
		w.Header()[key] = value
	}

	// set the content type header to application/json and write the status code and JSON data to the response writer
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

// readJSON decodes JSON data from a request body into a destination struct. It also validates the request body data. If the request body is empty or
// contains invalid JSON, or the JSON data does not match the structure of the destination struct, the method returns an error. If the request body
// contains a JSON array, or a JSON object with multiple keys, the method returns an error. The method also limits the size of the request body to 1MB.
func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {

	// limit the size of the request body to 1MB
	maxBytes := 1_048_576
	// use the MaxBytesReader() function to limit the size of the request body to 1MB. If the request body is larger than this, the server will respond with a 413 Payload Too Large response.
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))
	// initialize the json decoder, and call DisallowUnknownFields() method on it to and return and error for JSON fields which
	// cannot be matched to a destination instead of being silently ignored.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	// decode the request body into the destination struct. If there is an error, handle the error based on its type.
	err := dec.Decode(dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		// Use the errors.As() function to check whether the error has the type *json.SyntaxError.
		// If it does, return a plain-text error message containing the byte offset where the syntax error occurred.
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)
			// In some circumstances, Decode() may also return an io.ErrUnexpectedEOF error for syntax errors in the JSON.
			// If this happens, you should return a plain-text error message that the body contains badly-formed JSON.
		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")
			// we catch any *json.UnmarshalTypeError errors. These occur when the JSON value is the wrong type for the target destination.
			// If the error relates to a specific field in the struct, we include that in the error message.
		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}
			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)

		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
			// If the JSON contains a field which cannot be mapped to the target destination, Decode() will return an error message in the format 'json: unknown field "<field name>"'.
			// we check for this , extract the field name from the error, and interpolate it into a custom error message.
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)
			// If the request body exceeds the maximum allowed size, Decode() will return an error message in the format 'http: request body too large'.
		case err.Error() == "http: request body too large":
			return fmt.Errorf("body must not be larger than %d bytes", maxBytes)
		case errors.As(err, &invalidUnmarshalError):
			// TODO: Check for a better error message and return it, panic is not a good idea
			panic(err)

		default:
			return err
		}
	}

	// call decode again, using a pointer to an empty anonymous struct as the destination. If this succeeds, then the request body only contained a single JSON value.
	// if it fails, then the request body contained additional data, so return an error.
	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body must only contain a single JSON value")
	}
	return nil
}

// readString helper returns a string value from the query string, or the provided default value if no key is found.
func (app *application) readString(qs url.Values, key string, defaultValue string) string {
	// extract value for a key from the query string, if no key is exist, this will return empty string ""
	s := qs.Get(key)

	// if no key is found, return the default value
	if s == "" {
		return defaultValue
	}

	// otherwise return the string value
	return s
}

// readCSV helper returns a []string slice from the query string, or the provided default value if no key is found.
// This helper is used to extract comma-separated values from the query string. If the key is not found in the query string, the helper returns the provided default value.
func (app *application) readCSV(qs url.Values, key string, defaultValue []string) []string {
	// extract value for a key from the query string, if no key is exist, this will return empty string ""
	csv := qs.Get(key)

	if csv == "" {
		return defaultValue
	}

	// otherwise parse the value to a []string slice and return it
	return strings.Split(csv, ",")
}

// readInt helper returns an integer value from the query string, or the provided default value if no key is found.
func (app *application) readInt(qs url.Values, key string, defaultValue int, v *validator.Validator) int {
	// extract value for a key from the query string, if no key is exist, this will return empty string ""
	s := qs.Get(key)

	// if no key is found, return the default value
	if s == "" {
		return defaultValue
	}

	// try to convert the string value to an integer, if it fails, add an error to the validator and return the default value
	i, err := strconv.Atoi(s)
	if err != nil {
		v.AddError(key, "must be an integer value")
		return defaultValue
	}

	// otherwise return the converted integer value
	return i
}

// The background helper method is used to start a background goroutine for a given function. This is useful for running background tasks that do not need to block the main application thread.
// The method uses a deferred function to recover from any runtime panics and log the error using the application logger, instead of terminating the application.
func (app *application) background(fn func()) {
	// Increment the WaitGroup counter
	app.wg.Add(1)

	// Run a deferred function which uses recover() to catch any runtime panics and log the error using the application logger
	// instead of terminating the application
	go func() {

		// Use defer to decrement the WaitGroup counter before the goroutine returns
		defer app.wg.Done()

		defer func() {
			// Recover from any runtime panics and log the error using the application logger
			if err := recover(); err != nil {
				app.logger.PrintError(fmt.Errorf("%s", err), nil)
			}
		}()
		// Execute the arbitrary function that was passed in as an argument
		fn()
	}()
}
