package decimal

import "testing"

func TestCmp(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1", "2", -1},
		{"2", "1", 1},
		{"1", "1", 0},
		{"1.0", "1.00", 0},
		{"1.50", "1.5", 0},
		{"2E+5", "200000", 0},
		{"0", "-0", 0},
		{"0E+5", "0", 0},
		{"-1", "1", -1},
		{"-2", "-1", -1},
		{"431762.31", "431762.3", 1},
		{"0.0000001", "0", 1},
		{"-Infinity", "0", -1},
		{"Infinity", "1E+400", 1},
		{"-Infinity", "Infinity", -1},
		{"Infinity", "Infinity", 0},
	}
	for _, c := range cases {
		got, ok := MustParse(c.a).Cmp(MustParse(c.b))
		if !ok || got != c.want {
			t.Errorf("Cmp(%s, %s) = %d, %v; want %d, true", c.a, c.b, got, ok, c.want)
		}
	}
	if _, ok := MustParse("NaN").Cmp(MustParse("1")); ok {
		t.Error("Cmp(NaN, 1): ok = true; want false")
	}
	if _, ok := MustParse("1").Cmp(MustParse("sNaN")); ok {
		t.Error("Cmp(1, sNaN): ok = true; want false")
	}
}
