package api

import (
	"encoding/json"
	"fmt"
)

// JSON is a custom GraphQL type. It has to be added to a schema
// via "scalar JSON" since it is not a predeclared GraphQL type like "ID".
type JSON struct {
	Object interface{}
	msg    json.RawMessage
}

// ImplementsGraphQLType maps this custom Go type
// to the graphql scalar type in the schema.
func (JSON) ImplementsGraphQLType(name string) bool {
	return name == "JSON"
}

// UnmarshalGraphQL is a custom unmarshaler for Time
//
// This function will be called whenever you use the
// time scalar as an input
func (j *JSON) UnmarshalGraphQL(input interface{}) error {
	switch input := input.(type) {
	case json.RawMessage:
		j.msg = input
		return nil
	// case string:
	// 	json.Unmarshal([]byte(input), )
	// 	var err error
	// 	t.Time, err = time.Parse(time.RFC3339, input)
	// 	return err
	default:
		return fmt.Errorf("wrong type")
	}
}

// MarshalJSON is a custom marshaler for Time
//
// This function will be called whenever you
// query for fields that use the Time type
func (j JSON) MarshalJSON() ([]byte, error) {
	return json.Marshal(j.Object)
}
