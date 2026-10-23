package identity

import "testing"

func TestValidAccountID(t *testing.T) {
	for _, id := range []string{"1", "10", "9007199254740993", "9223372036854775807"} {
		if !ValidAccountID(id) {
			t.Errorf("rejected account ID %q", id)
		}
	}
	for _, id := range []string{"", "0", "-1", "+1", "01", " 1", "1 ", "1.0", "1e3", "１２", "9223372036854775808", "18446744073709551615", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"} {
		if ValidAccountID(id) {
			t.Errorf("accepted invalid account ID %q", id)
		}
	}
}
