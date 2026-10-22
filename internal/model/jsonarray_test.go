package model

import "testing"

func TestToJSONArray(t *testing.T) {
	var nilSlice []int
	cases := []struct {
		in   any
		want string
	}{
		{nilSlice, "[]"},
		{[]int{}, "[]"},
		{[]string{"a"}, `["a"]`},
		{[]map[string]int{{"seq": 1}}, `[{"seq":1}]`},
	}
	for _, c := range cases {
		if got := string(ToJSONArray(c.in)); got != c.want {
			t.Errorf("ToJSONArray(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}
