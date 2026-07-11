package decimal

import (
	"math/big"
	"testing"
)

// FuzzAddMulExact cross-checks Add and Mul against math/big.Rat, which is an
// exact oracle for these operations under an unlimited-precision context.
func FuzzAddMulExact(f *testing.F) {
	for _, s := range [][2]string{
		{"1.5", "2.25"}, {"0", "0"}, {"-3.30", "3.3"},
		{"123.456", "0.001"}, {"999999999", "1"}, {"-0.0010", "1E+3"},
	} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, sa, sb string) {
		a, err := NewFromString(sa)
		if err != nil {
			t.Skip()
		}
		b, err := NewFromString(sb)
		if err != nil {
			t.Skip()
		}
		if !a.IsFinite() || !b.IsFinite() { // big.Rat is an oracle for finite values only
			t.Skip()
		}
		if a.NumDigits() > 200 || b.NumDigits() > 200 || absExp(a) > 500 || absExp(b) > 500 {
			t.Skip()
		}
		sum, _ := Exact.Add(a, b)
		if sum.Rat().Cmp(new(big.Rat).Add(a.Rat(), b.Rat())) != 0 {
			t.Fatalf("Add(%s, %s) = %s: not exact", sa, sb, sum)
		}
		prod, _ := Exact.Mul(a, b)
		if prod.Rat().Cmp(new(big.Rat).Mul(a.Rat(), b.Rat())) != 0 {
			t.Fatalf("Mul(%s, %s) = %s: not exact", sa, sb, prod)
		}
	})
}

// absExp returns |exp| as an int64, so it is safe even for math.MinInt32.
func absExp(d Decimal) int64 {
	e := int64(d.Exp())
	if e < 0 {
		return -e
	}
	return e
}
