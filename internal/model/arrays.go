package model

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// StringArray is a []string that maps to PostgreSQL text[].
type StringArray []string

func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return "{}", nil
	}
	elems := make([]string, len(a))
	for i, s := range a {
		elems[i] = `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return "{" + strings.Join(elems, ",") + "}", nil
}

func (a *StringArray) Scan(src any) error {
	if src == nil {
		*a = StringArray{}
		return nil
	}
	var s string
	switch v := src.(type) {
	case []byte:
		s = string(v)
	case string:
		s = v
	default:
		return fmt.Errorf("StringArray.Scan: unsupported type %T", src)
	}
	s = strings.TrimSpace(s)
	if s == "{}" || s == "" {
		*a = StringArray{}
		return nil
	}
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	*a = parseArray(s)
	return nil
}

// Int64Array is a []int64 that maps to PostgreSQL bigint[].
type Int64Array []int64

func (a Int64Array) Value() (driver.Value, error) {
	if a == nil {
		return "{}", nil
	}
	elems := make([]string, len(a))
	for i, v := range a {
		elems[i] = strconv.FormatInt(v, 10)
	}
	return "{" + strings.Join(elems, ",") + "}", nil
}

func (a *Int64Array) Scan(src any) error {
	if src == nil {
		*a = Int64Array{}
		return nil
	}
	var s string
	switch v := src.(type) {
	case []byte:
		s = string(v)
	case string:
		s = v
	default:
		return fmt.Errorf("Int64Array.Scan: unsupported type %T", src)
	}
	s = strings.TrimSpace(s)
	if s == "{}" || s == "" {
		*a = Int64Array{}
		return nil
	}
	s = strings.TrimPrefix(s, "{")
	s = strings.TrimSuffix(s, "}")
	parts := strings.Split(s, ",")
	result := make(Int64Array, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return err
		}
		result = append(result, n)
	}
	*a = result
	return nil
}

func parseArray(s string) []string {
	var result []string
	var current strings.Builder
	inQuote := false
	escaped := false
	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inQuote = !inQuote
			continue
		}
		if r == ',' && !inQuote {
			result = append(result, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}
