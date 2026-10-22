package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONArray holds a JSON array for a jsonb column; JSONB only fits objects.
type JSONArray json.RawMessage

func ToJSONArray(v any) JSONArray {
	b, err := json.Marshal(v)
	if err != nil || string(b) == "null" {
		return JSONArray("[]")
	}
	return JSONArray(b)
}

func (a JSONArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "[]", nil
	}
	return string(a), nil
}

func (a *JSONArray) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*a = JSONArray("[]")
	case []byte:
		*a = append(JSONArray(nil), v...)
	case string:
		*a = JSONArray(v)
	default:
		return fmt.Errorf("JSONArray.Scan: unsupported type %T", src)
	}
	return nil
}

func (a JSONArray) MarshalJSON() ([]byte, error) {
	if len(a) == 0 {
		return []byte("[]"), nil
	}
	return a, nil
}

func (a *JSONArray) UnmarshalJSON(b []byte) error {
	*a = append(JSONArray(nil), b...)
	return nil
}
