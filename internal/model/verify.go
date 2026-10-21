package model

import "time"

const AttrLastVerifiedAt = "last_verified_at"

var timeFormats = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

func ParseLooseTime(s string) (time.Time, bool) {
	for _, f := range timeFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func VerifiedAt(it *Item) (time.Time, bool) {
	if it.Attributes == nil {
		return time.Time{}, false
	}
	v, ok := it.Attributes[AttrLastVerifiedAt]
	if !ok {
		return time.Time{}, false
	}
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	return ParseLooseTime(s)
}
