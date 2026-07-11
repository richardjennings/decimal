package decimal

import "testing"

var benchCtx = Context{Precision: 34, Rounding: RoundHalfEven}

// sink prevents the compiler from optimising benchmark results away.
var sink any

func BenchmarkAdd(b *testing.B) {
	x, y := MustParse("12345.678"), MustParse("9876.5432")
	for b.Loop() {
		sink, _ = benchCtx.Add(x, y)
	}
}

func BenchmarkMul(b *testing.B) {
	x, y := MustParse("12345.678"), MustParse("9876.5432")
	for b.Loop() {
		sink, _ = benchCtx.Mul(x, y)
	}
}

func BenchmarkQuantize(b *testing.B) {
	x, pat := MustParse("12345.6789"), New(1, -2)
	for b.Loop() {
		sink, _ = benchCtx.Quantize(x, pat)
	}
}

func BenchmarkParse(b *testing.B) {
	for b.Loop() {
		sink, _ = NewFromString("-12345.6789")
	}
}

func BenchmarkString(b *testing.B) {
	x := MustParse("12345.6789")
	for b.Loop() {
		sink = x.String()
	}
}
