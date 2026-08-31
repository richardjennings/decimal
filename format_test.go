package decimal

import "testing"

func TestStringPlain(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0", "0"},
		{"-0", "-0"},
		{"7", "7"},
		{"7.0", "7.0"},
		{"1.50", "1.50"},
		{"-431762.31", "-431762.31"},
		{"2E+5", "200000"},
		{"12345E+2", "1234500"},
		{"1.5E-3", "0.0015"},
		{"5E-7", "0.0000005"},
		{"123.456", "123.456"},
		{"0E+5", "0"},
		{"0E-2", "0.00"},
		{"-0E+3", "-0"},
		{"Infinity", "Infinity"},
		{"-Infinity", "-Infinity"},
		{"NaN", "NaN"},
	}
	for _, c := range cases {
		if got := MustParse(c.in).StringPlain(); got != c.want {
			t.Errorf("StringPlain(%s) = %q; want %q", c.in, got, c.want)
		}
	}
}
