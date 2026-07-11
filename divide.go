package decimal

import (
	"math"
	"math/big"
)

func clampExp(e int64) int32 {
	switch {
	case e > math.MaxInt32:
		return math.MaxInt32
	case e < math.MinInt32:
		return math.MinInt32
	}
	return int32(e)
}

func zeroExp(neg bool, exp int64) Decimal {
	z := New(0, clampExp(exp))
	z.neg = neg
	return z
}

// Divide returns a / b rounded to the context.
func (c Context) Divide(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	sign := a.sign() != b.sign()
	switch {
	case a.isInf() && b.isInf():
		return makeNaN(false, nil, false), InvalidOperation
	case a.isInf():
		return infinity(sign), 0
	case b.isInf():
		e := int64(0)
		if c.Bounded {
			e = math.MinInt32 // finalize clamps up to Etiny and raises Clamped
		}
		return c.finalize(zeroExp(sign, e), 0)
	case b.isZero():
		if a.isZero() {
			return makeNaN(false, nil, false), DivisionUndefined
		}
		return infinity(sign), DivisionByZero
	case a.isZero():
		return c.finalize(zeroExp(sign, int64(a.exp)-int64(b.exp)), 0)
	}
	return c.divideFinite(a, b, sign)
}

// divideFinite performs long division of two finite non-zero magnitudes,
// producing a quotient of at most prec significant digits with correct rounding.
func (c Context) divideFinite(a, b Decimal, sign bool) (Decimal, Condition) {
	prec := c.Precision
	if prec <= 0 {
		prec = 34 // fallback when the context is unlimited
	}
	ca := new(big.Int).Abs(&a.coeff)
	cb := new(big.Int).Abs(&b.coeff)
	ideal := int64(a.exp) - int64(b.exp)

	// Scale the dividend/divisor so the integer quotient has prec (or prec+1) digits.
	s := int(prec) - (numDigits(ca) - numDigits(cb))
	num := new(big.Int).Set(ca)
	den := new(big.Int).Set(cb)
	if s > 0 {
		num.Mul(num, pow10(uint(s)))
	} else if s < 0 {
		den.Mul(den, pow10(uint(-s)))
	}
	q := new(big.Int)
	r := new(big.Int)
	q.QuoRem(num, den, r)
	exp := ideal - int64(s)
	if int32(numDigits(q)) > prec { // one digit too many
		den.Mul(den, big.NewInt(10))
		q.QuoRem(num, den, r)
		exp++
	}

	// A subnormal result would double-round (here and in finalize); recompute
	// with guard digits and round once.
	if etiny := int64(c.MinExponent) - int64(c.Precision) + 1; c.Bounded && r.Sign() != 0 && exp < etiny {
		return c.divideSubnormal(ca, cb, ideal, sign)
	}

	var cond Condition
	if r.Sign() != 0 {
		cond = Inexact | Rounded
		if roundUp(q, r, den, sign, c.Rounding) {
			q.Add(q, big.NewInt(1))
			if int32(numDigits(q)) > prec { // rounding carry, e.g. 999 -> 1000
				q.Quo(q, big.NewInt(10))
				exp++
			}
		}
	} else {
		// Exact: raise the exponent toward the ideal, stripping trailing zeros.
		ten := big.NewInt(10)
		quo := new(big.Int)
		mod := new(big.Int)
		for exp < ideal {
			quo.QuoRem(q, ten, mod)
			if mod.Sign() != 0 {
				break
			}
			q.Set(quo)
			exp++
		}
		if exp > ideal { // coefficient was shortened to fit precision
			cond = Rounded
		}
	}

	res := newDec(q, clampExp(exp))
	if sign {
		res = res.Neg()
	}
	return c.finalize(res, cond)
}

// divideSubnormal computes a/b to precision+2 guard digits, encodes the division
// remainder as a round-to-odd sticky, and rounds exactly once via roundFinite.
func (c Context) divideSubnormal(ca, cb *big.Int, ideal int64, sign bool) (Decimal, Condition) {
	s := int(c.Precision) + 2 - (numDigits(ca) - numDigits(cb))
	num := new(big.Int).Set(ca)
	den := new(big.Int).Set(cb)
	if s > 0 {
		num.Mul(num, pow10(uint(s)))
	} else if s < 0 {
		den.Mul(den, pow10(uint(-s)))
	}
	q := new(big.Int)
	r := new(big.Int)
	q.QuoRem(num, den, r)
	exp := ideal - int64(s)
	if int64(numDigits(q)) > int64(c.Precision)+2 {
		den.Mul(den, big.NewInt(10))
		q.QuoRem(num, den, r)
		exp++
	}
	if r.Sign() != 0 && q.Bit(0) == 0 {
		q.Add(q, big.NewInt(1)) // round-to-odd sticky
	}
	if sign {
		q.Neg(q)
	}
	return c.roundFinite(q, exp, 0)
}

func adjExp(d Decimal) int64 { return int64(d.exp) + int64(numDigits(&d.coeff)) - 1 }

// alignedA expresses |a| at exponent min(a.exp, b.exp). It is only called when
// adjExp(a) < adjExp(b), which bounds the shift by the operand digit counts and
// so never materialises a huge pow10.
func alignedA(a, b Decimal) (*big.Int, int32) {
	m := new(big.Int).Abs(&a.coeff)
	e := a.exp
	if b.exp < a.exp {
		m.Mul(m, pow10(uint(int64(a.exp)-int64(b.exp))))
		e = b.exp
	}
	return m, e
}

// intDivModBounded is intDivMod with guards that avoid aligning operands whose
// exponents span more than the precision allows: when the integer quotient would
// exceed precision it returns ok=false (Division_impossible), and when |a| < |b|
// the quotient is zero so the remainder is a, computed without a huge alignment.
func (c Context) intDivModBounded(a, b Decimal) (q, rem *big.Int, remExp int32, ok bool) {
	if c.Precision > 0 && adjExp(a)-adjExp(b) > int64(c.Precision) {
		return nil, nil, 0, false
	}
	if adjExp(a) < adjExp(b) {
		rem, e := alignedA(a, b)
		return big.NewInt(0), rem, e, true
	}
	ca, cb, e := align(a, b)
	q = new(big.Int)
	rem = new(big.Int)
	q.QuoRem(new(big.Int).Abs(ca), new(big.Int).Abs(cb), rem)
	return q, rem, e, true
}

// DivideInt returns the integer part of a / b (truncated toward zero, exponent 0).
func (c Context) DivideInt(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	sign := a.sign() != b.sign()
	switch {
	case a.isInf() && b.isInf():
		return makeNaN(false, nil, false), InvalidOperation
	case a.isInf():
		return infinity(sign), 0
	case b.isZero():
		if a.isZero() {
			return makeNaN(false, nil, false), DivisionUndefined
		}
		return infinity(sign), DivisionByZero
	case b.isInf(), a.isZero():
		return zeroExp(sign, 0), 0
	}
	q, _, _, ok := c.intDivModBounded(a, b)
	if !ok || (c.Precision > 0 && int32(numDigits(q)) > c.Precision) {
		return makeNaN(false, nil, false), DivisionImpossible
	}
	res := newDec(q, 0)
	if q.Sign() == 0 {
		res.neg = sign
	} else if sign {
		res = res.Neg()
	}
	return res, 0
}

// Rem returns the remainder of a / b (a - divideint(a,b)*b); sign of the dividend.
func (c Context) Rem(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	switch {
	case a.isInf():
		return makeNaN(false, nil, false), InvalidOperation
	case b.isZero():
		if a.isZero() {
			return makeNaN(false, nil, false), DivisionUndefined
		}
		return makeNaN(false, nil, false), InvalidOperation
	case b.isInf():
		return c.RoundToContext(a)
	case a.isZero():
		e := a.exp
		if b.exp < e {
			e = b.exp
		}
		z := New(0, e)
		z.neg = a.sign()
		return c.RoundToContext(z)
	}
	q, rem, remExp, ok := c.intDivModBounded(a, b)
	if !ok || (c.Precision > 0 && int32(numDigits(q)) > c.Precision) {
		return makeNaN(false, nil, false), DivisionImpossible
	}
	res := newDec(rem, remExp)
	if a.sign() {
		if rem.Sign() != 0 {
			res = res.Neg()
		} else {
			res.neg = true
		}
	}
	return c.RoundToContext(res)
}

// RemNear returns a - round-half-even(a/b)*b (IEEE remainder); may be negative for positive a.
func (c Context) RemNear(a, b Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a, b); ok {
		return r, cond
	}
	switch {
	case a.isInf():
		return makeNaN(false, nil, false), InvalidOperation
	case b.isZero():
		if a.isZero() {
			return makeNaN(false, nil, false), DivisionUndefined
		}
		return makeNaN(false, nil, false), InvalidOperation
	case b.isInf():
		return c.RoundToContext(a)
	case a.isZero():
		e := a.exp
		if b.exp < e {
			e = b.exp
		}
		z := New(0, e)
		z.neg = a.sign()
		return c.RoundToContext(z)
	}
	if c.Precision > 0 && adjExp(a)-adjExp(b) > int64(c.Precision) {
		return makeNaN(false, nil, false), DivisionImpossible
	}
	if adjExp(a)+1 < adjExp(b) { // |a| < |b|/10: nearest quotient is 0, remainder is a
		m, e := alignedA(a, b)
		res := newDec(m, e)
		if a.sign() {
			if m.Sign() != 0 {
				res = res.Neg()
			} else {
				res.neg = true
			}
		}
		return c.RoundToContext(res)
	}
	ca, cb, e := align(a, b)
	maca := new(big.Int).Abs(ca)
	macb := new(big.Int).Abs(cb)
	q := new(big.Int)
	rf := new(big.Int)
	q.QuoRem(maca, macb, rf)
	if cmp := new(big.Int).Lsh(rf, 1).Cmp(macb); cmp > 0 || (cmp == 0 && q.Bit(0) == 1) {
		q.Add(q, big.NewInt(1))
	}
	if c.Precision > 0 && int32(numDigits(q)) > c.Precision {
		return makeNaN(false, nil, false), DivisionImpossible
	}
	coeff := new(big.Int).Sub(maca, new(big.Int).Mul(q, macb))
	if a.sign() {
		coeff.Neg(coeff)
	}
	res := newDec(coeff, e)
	if coeff.Sign() == 0 {
		res.neg = a.sign()
	}
	return c.RoundToContext(res)
}
