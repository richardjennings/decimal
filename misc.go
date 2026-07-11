package decimal

import (
	"math"
	"math/big"
)

// FMA returns (a*b)+x with the multiplication carried out exactly before the
// single rounding of the addition.
func (c Context) FMA(a, b, x Decimal) (Decimal, Condition) {
	invalidMul := (a.isInf() && b.isZero()) || (b.isInf() && a.isZero())
	if r, cond, ok := nanResult(c.Precision, a, b, x); ok {
		if invalidMul {
			cond |= InvalidOperation // 0 x Inf is invalid even alongside a quiet NaN
		}
		return r, cond // NaN priority runs left-to-right across all three operands
	}
	prod, cond := Exact.Mul(a, b)
	res, k := c.Add(prod, x)
	return res, cond | k
}

// --- logical operations: operands must be non-negative, exponent 0, digits 0/1 ---

func logicalDigits(d Decimal) (string, bool) {
	if d.form != finite || d.exp != 0 || d.sign() {
		return "", false
	}
	s := new(big.Int).Abs(&d.coeff).Text(10)
	for i := 0; i < len(s); i++ {
		if s[i] != '0' && s[i] != '1' {
			return "", false
		}
	}
	return s, true
}

func bitwise(a, b Decimal, prec int, op func(x, y byte) byte) (Decimal, Condition) {
	da, oka := logicalDigits(a)
	db, okb := logicalDigits(b)
	if !oka || !okb {
		return makeNaN(false, nil, false), InvalidOperation
	}
	n := min(max(len(da), len(db)), prec) // operands are truncated to the rightmost prec digits
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		x, y := byte('0'), byte('0')
		if i < len(da) {
			x = da[len(da)-1-i]
		}
		if i < len(db) {
			y = db[len(db)-1-i]
		}
		buf[n-1-i] = op(x, y)
	}
	coeff, _ := new(big.Int).SetString(string(buf), 10)
	return newDec(coeff, 0), 0
}

func (c Context) And(a, b Decimal) (Decimal, Condition) {
	return bitwise(a, b, int(c.Precision), func(x, y byte) byte {
		if x == '1' && y == '1' {
			return '1'
		}
		return '0'
	})
}

func (c Context) Or(a, b Decimal) (Decimal, Condition) {
	return bitwise(a, b, int(c.Precision), func(x, y byte) byte {
		if x == '1' || y == '1' {
			return '1'
		}
		return '0'
	})
}

func (c Context) Xor(a, b Decimal) (Decimal, Condition) {
	return bitwise(a, b, int(c.Precision), func(x, y byte) byte {
		if x != y {
			return '1'
		}
		return '0'
	})
}

// Invert inverts each of the precision digits of a (a logical NOT).
func (c Context) Invert(a Decimal) (Decimal, Condition) {
	da, ok := logicalDigits(a)
	if !ok {
		return makeNaN(false, nil, false), InvalidOperation
	}
	n := int(c.Precision)
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		x := byte('0')
		if i < len(da) {
			x = da[len(da)-1-i]
		}
		if x == '0' {
			buf[n-1-i] = '1'
		} else {
			buf[n-1-i] = '0'
		}
	}
	coeff, _ := new(big.Int).SetString(string(buf), 10)
	return newDec(coeff, 0), 0
}

// --- digit shift / rotate within the precision window ---

// padDigits returns the magnitude of d as exactly n digits (left-zero-padded,
// or the low n digits if longer).
func padDigits(d Decimal, n int) []byte {
	s := new(big.Int).Abs(&d.coeff).Text(10)
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = '0'
	}
	for i := 0; i < len(s) && i < n; i++ {
		buf[n-1-i] = s[len(s)-1-i]
	}
	return buf
}

// shiftCount validates a shift/rotate count operand: a plain integer in the
// range [-precision, precision].
func shiftCount(b Decimal, prec int32) (int, bool) {
	if b.form != finite || b.exp != 0 || !b.coeff.IsInt64() {
		return 0, false
	}
	n := b.coeff.Int64()
	if n < int64(-prec) || n > int64(prec) {
		return 0, false
	}
	return int(n), true
}

func (c Context) Shift(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	n, ok := shiftCount(b, c.Precision)
	if !ok {
		return makeNaN(false, nil, false), InvalidOperation
	}
	if a.isInf() {
		return a, 0
	}
	digits := padDigits(a, int(c.Precision))
	p := int(c.Precision)
	out := make([]byte, p)
	for i := range out {
		out[i] = '0'
	}
	if n >= 0 { // shift left: digit at j moves to j-n
		for j := 0; j < p; j++ {
			if j-n >= 0 {
				out[j-n] = digits[j]
			}
		}
	} else {
		for j := 0; j < p; j++ {
			if j-n < p {
				out[j-n] = digits[j]
			}
		}
	}
	return shiftedResult(a, out), 0
}

func (c Context) Rotate(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	nRaw, ok := shiftCount(b, c.Precision)
	if !ok {
		return makeNaN(false, nil, false), InvalidOperation
	}
	if a.isInf() {
		return a, 0
	}
	p := int(c.Precision)
	n := ((nRaw % p) + p) % p // normalize into [0,p)
	digits := padDigits(a, p)
	out := make([]byte, p)
	for j := 0; j < p; j++ {
		out[(j-n+p)%p] = digits[j] // rotate left by n
	}
	return shiftedResult(a, out), 0
}

// shiftedResult rebuilds a Decimal from shifted/rotated digits, preserving the
// sign and exponent of the original.
func shiftedResult(a Decimal, digits []byte) Decimal {
	coeff, _ := new(big.Int).SetString(string(digits), 10)
	res := newDec(coeff, a.exp)
	if a.sign() {
		if coeff.Sign() != 0 {
			res = res.Neg()
		} else {
			res.neg = true
		}
	}
	return res
}

// ScaleB returns a * 10^b, where b is an integer, rounded to the context.
func (c Context) ScaleB(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	if b.form != finite || b.exp != 0 || !b.coeff.IsInt64() {
		return makeNaN(false, nil, false), InvalidOperation
	}
	n := b.coeff.Int64()
	if c.MaxExponent != 0 {
		if limit := 2 * (int64(c.MaxExponent) + int64(c.Precision)); n > limit || n < -limit {
			return makeNaN(false, nil, false), InvalidOperation
		}
	}
	if a.isInf() {
		return a, 0
	}
	in := newDec(&a.coeff, clampExp(int64(a.exp)+n))
	in.neg = a.neg // preserve signed zero
	return c.RoundToContext(in)
}

// ToIntegral rounds a to an integer using the context rounding. When exact is
// true (to-integral-exact) it raises Inexact/Rounded; otherwise it is silent.
func (c Context) ToIntegral(a Decimal, exact bool) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isInf() {
		return a, 0
	}
	if a.exp >= 0 { // already integral; still fold the exponent down under a fixed format
		return c.finalize(a, 0)
	}
	var res Decimal
	var cond Condition
	if drop := -int64(a.exp); drop > int64(numDigits(&a.coeff)) {
		res = c.underflowToZero(a, 0) // |value| < 1: integral part rounds to 0 (or +/-1)
		if a.coeff.Sign() != 0 {
			cond = Inexact | Rounded
		}
	} else {
		q, _, k := shift(&a.coeff, int32(drop), c.Rounding)
		res = newDec(q, 0)
		if q.Sign() == 0 {
			res.neg = a.sign()
		}
		cond = k
	}
	if !exact {
		cond = 0
	}
	return res, cond
}

// LogB returns the adjusted exponent of a as an integer (floor(log10|a|)).
func (c Context) LogB(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isInf() {
		return infinity(false), 0
	}
	if a.isZero() {
		return infinity(true), DivisionByZero
	}
	adj := int64(a.exp) + int64(numDigits(&a.coeff)) - 1
	return c.RoundToContext(New(adj, 0))
}

// Reduce rounds a to the context and strips trailing zeros from the coefficient.
func (c Context) Reduce(a Decimal) (Decimal, Condition) {
	res, cond := c.RoundToContext(a)
	if res.form != finite {
		return res, cond
	}
	if res.coeff.Sign() == 0 {
		z := New(0, 0)
		z.neg = res.neg
		return z, cond
	}
	ten := big.NewInt(10)
	quo := new(big.Int)
	mod := new(big.Int)
	coeff := new(big.Int).Set(&res.coeff)
	exp := int64(res.exp)
	limit := int64(math.MaxInt32)
	if c.Bounded && c.Clamp { // don't strip past Etop, so no Clamped is raised
		limit = emaxEtop(c)
	}
	for exp < limit {
		quo.QuoRem(coeff, ten, mod)
		if mod.Sign() != 0 {
			break
		}
		coeff.Set(quo)
		exp++
	}
	return c.finalize(newDec(coeff, clampExp(exp)), cond)
}

// Canonical returns a unchanged (this representation is always canonical).
func (a Decimal) Canonical() Decimal { return a }

// Trim removes insignificant trailing zeros, raising the exponent up to (at most)
// zero, without changing the value.
func (a Decimal) Trim() Decimal {
	if a.form != finite {
		return a
	}
	if a.coeff.Sign() == 0 {
		z := New(0, 0)
		z.neg = a.sign()
		return z
	}
	coeff := new(big.Int).Set(&a.coeff)
	exp := int64(a.exp)
	stripAll := a.exp > 0 // an already-positive exponent keeps rising
	ten := big.NewInt(10)
	quo := new(big.Int)
	mod := new(big.Int)
	for stripAll || exp < 0 {
		quo.QuoRem(coeff, ten, mod)
		if mod.Sign() != 0 {
			break
		}
		coeff.Set(quo)
		exp++
	}
	return newDec(coeff, int32(exp))
}

// Class returns the GDA number class of a (e.g. "+Normal", "-Subnormal", "NaN").
func (c Context) Class(a Decimal) string {
	switch a.form {
	case signalingNaN:
		return "sNaN"
	case quietNaN:
		return "NaN"
	}
	sign := "+"
	if a.sign() {
		sign = "-"
	}
	switch {
	case a.form == infinite:
		return sign + "Infinity"
	case a.coeff.Sign() == 0:
		return sign + "Zero"
	case c.Bounded && int64(a.exp)+int64(numDigits(&a.coeff))-1 < int64(c.MinExponent):
		return sign + "Subnormal"
	default:
		return sign + "Normal"
	}
}

// integerValue returns d's value as an int64 when d is a whole number.
func (d Decimal) integerValue() (int64, bool) {
	if d.form != finite {
		return 0, false
	}
	if d.coeff.Sign() == 0 {
		return 0, true // zero, whatever the exponent
	}
	v := new(big.Int).Abs(&d.coeff)
	switch {
	case d.exp > 0:
		if int64(numDigits(&d.coeff))+int64(d.exp) > 18 {
			return 0, false // too large for int64
		}
		v.Mul(v, pow10(uint(d.exp)))
	case d.exp < 0:
		if int64(-d.exp) >= int64(numDigits(&d.coeff)) {
			return 0, false // more fractional places than digits: not a whole number
		}
		r := new(big.Int)
		v.QuoRem(v, pow10(uint(-d.exp)), r)
		if r.Sign() != 0 {
			return 0, false // not a whole number
		}
	}
	if d.sign() {
		v.Neg(v)
	}
	if !v.IsInt64() {
		return 0, false
	}
	return v.Int64(), true
}

// Rescale sets a's exponent to the integer value of b (the pre-quantize spelling
// of Quantize, where the pattern is given as an explicit exponent).
func (c Context) Rescale(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	if b.isInf() {
		return c.Quantize(a, b) // quantize(a, Inf): Infinity if a is infinite, else Invalid
	}
	target, ok := b.integerValue()
	if !ok {
		return makeNaN(false, nil, false), InvalidOperation
	}
	return c.Quantize(a, New(0, clampExp(target)))
}
