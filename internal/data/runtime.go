package data

import (
	"fmt"
	"strconv"
)

type Runtime int32 // define a custom type for runtime

// implement the MarshalJSON method on the Runtime type so that it satisfies the json.Marshaler interface
func (r Runtime) MarshalJSON() ([]byte, error) {
	jsonValue := fmt.Sprintf("%d mins", r)

	quotedJSONValue := strconv.Quote(jsonValue)
	return []byte(quotedJSONValue), nil

}
