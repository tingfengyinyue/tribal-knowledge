package store

import "testing"

func TestOrTSQuery(t *testing.T) {
	cases := map[string]string{
		"checkout fails since 10:00 (E4021) & retry!": "checkout | fails | since | 10 | 00 | e4021 | retry",
		"Wallet wallet WALLET":                        "wallet",
		"!!! ::: &&&":                                 "",
		"checkout 服务报错":                               "checkout | 服务报错",
	}
	for in, want := range cases {
		if got := orTSQuery(in); got != want {
			t.Errorf("orTSQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
