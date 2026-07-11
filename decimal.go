// Package decimal is an arbitrary-precision decimal arithmetic implementation
// built toward conformance with the General Decimal Arithmetic specification
// (https://speleotrove.com/decimal/). A finite value is coeff × 10^exp.
package decimal

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type form uint8

const (
	finite form = iota
	infinite
	quietNaN
	signalingNaN
)

// Decimal is an immutable decimal number. Operations return new values and never
// mutate the coefficient in place, so copying a Decimal is safe.
type Decimal struct {
	form  form
	neg   bool    // sign for non-finite forms (finite sign lives in coeff)
	coeff big.Int // coefficient (finite: signed value; NaN: payload magnitude)
	exp   int32
}

// New returns the finite value coeff × 10^exp.
func New(coeff int64, exp int32) Decimal {
	var d Decimal
	d.coeff.SetInt64(coeff)
	d.exp = exp
	return d
}

func newDec(coeff *big.Int, exp int32) Decimal {
	var d Decimal
	d.coeff.Set(coeff)
	d.exp = exp
	return d
}

// NewFromString parses a decimal in GDA numeric-string syntax, including the
// special values NaN, sNaN, and Infinity.
func NewFromString(s string) (Decimal, error) {
	orig := s
	if s == "" {
		return Decimal{}, fmt.Errorf("decimal: empty string")
	}
	if sd, ok := parseSpecial(s); ok {
		return sd, nil
	}

	mant := s
	var expPart int64
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant = s[:i]
		e, err := strconv.ParseInt(s[i+1:], 10, 64)
		if err != nil {
			// A magnitude beyond int64 saturates (ErrRange) and still overflows or
			// underflows any context; only a malformed exponent is a syntax error.
			if ne, ok := err.(*strconv.NumError); !ok || ne.Err != strconv.ErrRange {
				return Decimal{}, fmt.Errorf("decimal: bad exponent in %q", orig)
			}
		}
		expPart = e
	}

	neg := false
	switch {
	case strings.HasPrefix(mant, "-"):
		neg, mant = true, mant[1:]
	case strings.HasPrefix(mant, "+"):
		mant = mant[1:]
	}

	intPart, fracPart := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, fracPart = mant[:i], mant[i+1:]
	}
	digits := intPart + fracPart
	if digits == "" {
		return Decimal{}, fmt.Errorf("decimal: no digits in %q", orig)
	}
	for i := 0; i < len(digits); i++ { // reject signs, spaces, and other stray characters
		if digits[i] < '0' || digits[i] > '9' {
			return Decimal{}, fmt.Errorf("decimal: invalid number %q", orig)
		}
	}
	var coeff big.Int
	coeff.SetString(digits, 10)
	if neg {
		coeff.Neg(&coeff)
	}
	// Clamp the raw exponent into int32 range before adjusting for the fraction, so
	// an astronomically large literal stays finite and simply overflows or
	// underflows when a bounded operation rounds it.
	exp := int64(clampExp(expPart)) - int64(len(fracPart))
	res := newDec(&coeff, clampExp(exp))
	if coeff.Sign() == 0 && neg {
		res.neg = true // negative zero
	}
	return res, nil
}

// MustParse is NewFromString that panics on error; for tests and constants.
func MustParse(s string) Decimal {
	d, err := NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func (d Decimal) IsFinite() bool { return d.form == finite }
func (d Decimal) Exp() int32     { return d.exp }
func (d Decimal) NumDigits() int { return numDigits(&d.coeff) }

// Signbit reports whether d is negative, including negative zero and -NaN.
func (d Decimal) Signbit() bool { return d.sign() }

// Coeff returns a copy of the coefficient.
func (d Decimal) Coeff() *big.Int { return new(big.Int).Set(&d.coeff) }

// Rat returns the exact value as a big.Rat (finite values only).
func (d Decimal) Rat() *big.Rat {
	r := new(big.Rat).SetInt(&d.coeff)
	if d.exp >= 0 {
		r.Mul(r, new(big.Rat).SetInt(pow10(uint(d.exp))))
	} else {
		r.Quo(r, new(big.Rat).SetInt(pow10(uint(-d.exp))))
	}
	return r
}

// Neg returns -d.
func (d Decimal) Neg() Decimal {
	if d.form != finite {
		nd := d
		nd.neg = !d.neg
		return nd
	}
	if d.coeff.Sign() == 0 {
		z := New(0, d.exp)
		z.neg = !d.sign()
		return z
	}
	var n big.Int
	n.Neg(&d.coeff)
	return newDec(&n, d.exp)
}

// Identical reports whether a and b have the same form, coefficient, and
// exponent, so that 1.0 and 1.00 are distinct — the equality GDA uses.
func Identical(a, b Decimal) bool {
	return a.form == b.form && a.neg == b.neg && a.exp == b.exp && a.coeff.Cmp(&b.coeff) == 0
}

// align scales a and b to a common exponent min(a.exp, b.exp).
func align(a, b Decimal) (ca, cb *big.Int, e int32) {
	e = a.exp
	if b.exp < e {
		e = b.exp
	}
	ca = new(big.Int).Set(&a.coeff)
	cb = new(big.Int).Set(&b.coeff)
	if d := a.exp - e; d > 0 {
		ca.Mul(ca, pow10(uint(d)))
	}
	if d := b.exp - e; d > 0 {
		cb.Mul(cb, pow10(uint(d)))
	}
	return
}

// Add returns a + b under the context.
func (c Context) Add(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	if a.isInf() || b.isInf() {
		switch {
		case a.isInf() && b.isInf():
			if a.neg != b.neg {
				return makeNaN(false, nil, false), InvalidOperation
			}
			return infinity(a.neg), 0
		case a.isInf():
			return infinity(a.neg), 0
		default:
			return infinity(b.neg), 0
		}
	}
	zeroSum := func(e int32) (Decimal, Condition) {
		res := New(0, e)
		res.neg = zeroAddSign(a, b, c.Rounding)
		return c.finalize(res, 0)
	}
	if c.Precision <= 0 { // unlimited: exact alignment is required
		ca, cb, e := align(a, b)
		sum := ca.Add(ca, cb)
		if sum.Sign() == 0 {
			return zeroSum(e)
		}
		return c.roundFinite(sum, int64(e), 0)
	}
	// Bounded alignment keeps only the digits that can affect a precision-rounded
	// result, so a billion-place exponent span never materialises pow10(span).
	keep := int64(a.NumDigits())
	if n := int64(b.NumDigits()); n > keep {
		keep = n
	}
	keep += int64(c.Precision) + 10
	ca, cb, e, sticky := alignBounded(a, b, keep)
	sum := ca.Add(ca, cb)
	if !sticky {
		if sum.Sign() == 0 {
			return zeroSum(e)
		}
		return c.roundFinite(sum, int64(e), 0)
	}
	sum = roundToOddTail(sum, smallerNeg(a, b))
	return c.roundFinite(sum, int64(e), 0)
}

// Sub returns a - b under the context.
func (c Context) Sub(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond // check before negating, so a NaN operand keeps its sign
	}
	return c.Add(a, b.Neg())
}

// Mul returns a * b under the context.
func (c Context) Mul(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	if a.isInf() || b.isInf() {
		if a.isZero() || b.isZero() {
			return makeNaN(false, nil, false), InvalidOperation
		}
		return infinity(a.sign() != b.sign()), 0
	}
	prod := new(big.Int).Mul(&a.coeff, &b.coeff)
	pe := int64(a.exp) + int64(b.exp)
	if prod.Sign() == 0 {
		res := New(0, clampExp(pe))
		res.neg = a.sign() != b.sign()
		return c.finalize(res, 0)
	}
	return c.roundFinite(prod, pe, 0)
}

// Plus returns a rounded to the context (unary +).
func (c Context) Plus(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isInf() {
		return infinity(a.neg), 0
	}
	if a.coeff.Sign() == 0 {
		res := New(0, a.exp)
		res.neg = a.sign() && c.Rounding == RoundFloor // plus(a) == add(+0, a)
		return c.finalize(res, 0)
	}
	return c.roundFinite(&a.coeff, int64(a.exp), 0)
}

// Minus returns -a rounded to the context (unary -).
func (c Context) Minus(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	return c.Plus(a.Neg())
}

// RoundToContext rounds a to the context precision, preserving its sign (unlike
// Plus/Minus, which follow the add-zero sign rule). This is GDA's apply.
func (c Context) RoundToContext(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isInf() {
		return infinity(a.neg), 0
	}
	if a.coeff.Sign() == 0 {
		res := New(0, a.exp)
		res.neg = a.sign()
		return c.finalize(res, 0)
	}
	return c.roundFinite(&a.coeff, int64(a.exp), 0)
}

// Abs returns |a| rounded to the context.
func (c Context) Abs(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isInf() {
		return infinity(false), 0
	}
	if a.sign() {
		return c.Plus(a.Neg())
	}
	return c.Plus(a)
}

// Compare returns -1, 0, or 1 as a Decimal (GDA compare). A NaN operand yields
// NaN; a signaling NaN also raises Invalid_operation.
func (c Context) Compare(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	return New(int64(cmpDecimal(a, b)), 0), 0
}

// Quantize rescales a to the exponent of pattern, rounding under the context.
func (c Context) Quantize(a, pattern Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, pattern); ok {
		return r, cond
	}
	if a.isInf() || pattern.isInf() {
		if a.isInf() && pattern.isInf() {
			return infinity(a.neg), 0
		}
		return makeNaN(false, nil, false), InvalidOperation
	}
	targetExp := pattern.exp
	var res Decimal
	var cond Condition
	switch {
	case a.exp == targetExp:
		res = newDec(&a.coeff, targetExp)
	case a.exp > targetExp:
		up := int64(a.exp) - int64(targetExp)
		if a.coeff.Sign() != 0 && c.Precision > 0 && int64(numDigits(&a.coeff))+up > int64(c.Precision) {
			return makeNaN(false, nil, false), InvalidOperation // would exceed precision
		}
		if a.coeff.Sign() == 0 {
			res = newDec(&a.coeff, targetExp)
		} else {
			res = newDec(new(big.Int).Mul(&a.coeff, pow10(uint(up))), targetExp)
		}
	default:
		if drop := int64(targetExp) - int64(a.exp); drop > int64(numDigits(&a.coeff)) {
			res = c.underflowToZero(a, int64(targetExp)) // avoids materialising pow10(drop)
			if a.coeff.Sign() != 0 {
				cond = Inexact | Rounded
			}
		} else {
			q, _, k := shift(&a.coeff, int32(drop), c.Rounding)
			res, cond = newDec(q, targetExp), k
		}
	}
	if res.coeff.Sign() == 0 {
		res.neg = a.sign()
	}
	if c.MaxExponent != 0 {
		adj := int64(targetExp) + int64(res.NumDigits()) - 1
		etiny := int64(c.MinExponent) - int64(c.Precision-1)
		if adj > int64(c.MaxExponent) || int64(targetExp) < etiny {
			return makeNaN(false, nil, false), InvalidOperation
		}
	}
	if c.Precision > 0 && int32(res.NumDigits()) > c.Precision {
		return makeNaN(false, nil, false), InvalidOperation
	}
	return res, cond | c.subnormalFlag(res)
}
