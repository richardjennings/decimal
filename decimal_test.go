package decimal

import "testing"

func TestNewFromString(t *testing.T) {
	cases := []struct {
		in    string
		coeff string
		exp   int32
	}{
		{"0", "0", 0},
		{"1.00", "100", -2},
		{"-12.5", "-125", -1},
		{"1E+3", "1", 3},
		{".5", "5", -1},
		{"1.", "1", 0},
		{"007", "7", 0},
		{"-0.0010", "-10", -4},
	}
	for _, c := range cases {
		d, err := NewFromString(c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if d.Coeff().String() != c.coeff || d.Exp() != c.exp {
			t.Errorf("%q: got (%s, %d), want (%s, %d)", c.in, d.Coeff(), d.Exp(), c.coeff, c.exp)
		}
	}
}

func TestArithmetic(t *testing.T) {
	cases := []struct {
		op   string
		a, b string
		want string
	}{
		{"add", "1.1", "2.2", "3.3"},
		{"add", "1.00", "2.000", "3.000"},
		{"sub", "5", "3.3", "1.7"},
		{"mul", "1.5", "1.5", "2.25"},
		{"mul", "-2", "2.5", "-5.0"},
	}
	for _, c := range cases {
		a, b := MustParse(c.a), MustParse(c.b)
		var got Decimal
		switch c.op {
		case "add":
			got, _ = Exact.Add(a, b)
		case "sub":
			got, _ = Exact.Sub(a, b)
		case "mul":
			got, _ = Exact.Mul(a, b)
		}
		if got.String() != c.want {
			t.Errorf("%s(%s, %s) = %s, want %s", c.op, c.a, c.b, got, c.want)
		}
	}
}

func TestQuantizeRounding(t *testing.T) {
	cases := []struct {
		in      string
		exp     int32
		mode    RoundingMode
		want    string
		inexact bool
	}{
		{"2.345", -2, RoundHalfEven, "2.34", true},
		{"2.355", -2, RoundHalfEven, "2.36", true},
		{"2.345", -2, RoundHalfUp, "2.35", true},
		{"2.34", -2, RoundHalfEven, "2.34", false},
		{"2", -2, RoundHalfEven, "2.00", false},
	}
	for _, c := range cases {
		ctx := Context{Precision: 9, Rounding: c.mode}
		got, cond := ctx.Quantize(MustParse(c.in), New(1, c.exp))
		if got.String() != c.want || cond.Has(Inexact) != c.inexact {
			t.Errorf("quantize(%s, %d, %v) = %s inexact=%v, want %s inexact=%v",
				c.in, c.exp, c.mode, got, cond.Has(Inexact), c.want, c.inexact)
		}
	}
}
