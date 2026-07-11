package decimal

import "math/big"

// This file implements the transcendental functions exp, ln, log10, and power.
// Each computes at extra "guard" precision (using the decimal operations at an
// unbounded working context) and rounds once to the target precision. The
// working precision is padded for the error amplified by argument reduction.

func workContext(prec, extra int) Context {
	return Context{Precision: int32(prec + extra), Rounding: RoundHalfEven}
}

func absCmp(a, b Decimal) int { return cmpDecimal(a.CopyAbs(), b.CopyAbs()) }

// mathContextInvalid reports whether the context is too extreme for a
// mathematical function (exp/ln/log10/power). decNumber caps precision and the
// exponent limits at DEC_MAX_MATH; a context narrowed beyond that (but short of
// the maximal DEC_MAX_EMAX default, which denotes "unrestricted") raises
// Invalid_context.
func (c Context) mathContextInvalid() bool {
	const maxMath, maxEmax = 999999, 999999999
	over := func(v int64) bool { return v > maxMath && v < maxEmax }
	return over(int64(c.Precision)) || over(int64(c.MaxExponent)) || over(-int64(c.MinExponent))
}

// finishTranscendental rounds a computed value to the context; a finite
// transcendental result is always inexact.
func (c Context) finishTranscendental(v Decimal) (Decimal, Condition) {
	res, cond := c.RoundToContext(v)
	if res.form == finite {
		cond |= Inexact | Rounded
	}
	return res, cond
}

// negligible reports whether v is zero or smaller in magnitude than 10^exp.
func negligible(v Decimal, exp int) bool {
	return v.isZero() || int64(v.exp)+int64(numDigits(&v.coeff))-1 < int64(exp)
}

// Exp returns e**x rounded to the context.
func (c Context) Exp(x Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, x); ok {
		return r, cond
	}
	if c.mathContextInvalid() {
		return makeNaN(false, nil, false), InvalidContext
	}
	if x.isInf() {
		if x.neg {
			return New(0, 0), 0 // e**-inf = 0
		}
		return infinity(false), 0
	}
	if x.isZero() {
		return New(1, 0), 0
	}
	prec := int(c.Precision)
	if prec <= 0 {
		prec = 34
	}

	// Argument reduction: e**x = (e**(x/2^n))**(2^n), with |x/2^n| < 1.
	one := New(1, 0)
	half := New(5, -1)
	wc := workContext(prec, 24)
	r := x
	n := 0
	for absCmp(r, one) >= 0 {
		r, _ = wc.Mul(r, half)
		n++
	}
	wc = workContext(prec, 20+n) // pad for the error amplified by n squarings
	small := -(prec + 18 + n)

	// Taylor series: e**r = 1 + r + r^2/2! + r^3/3! + ...
	sum := one
	term := one
	for k := 1; k < 100000; k++ {
		term, _ = wc.Mul(term, r)
		term, _ = wc.Divide(term, New(int64(k), 0))
		sum, _ = wc.Add(sum, term)
		if negligible(term, small) {
			break
		}
	}
	for i := 0; i < n; i++ {
		sum, _ = wc.Mul(sum, sum)
	}
	return c.finishTranscendental(sum)
}

// lnWork computes ln(x) for finite x > 0 at the working context wc, via
// sqrt-reduction toward 1 followed by the atanh series.
func lnWork(wc Context, x Decimal, small int) Decimal {
	one := New(1, 0)
	two := New(2, 0)
	tenth := New(1, -1)

	// ln(x) = 2^m * ln(x^(1/2^m)); take square roots until x^(1/2^m) ~ 1.
	m := 0
	y := x
	for m < 100000 {
		if diff, _ := wc.Sub(y, one); absCmp(diff, tenth) < 0 {
			break
		}
		y, _ = wc.Sqrt(y)
		m++
	}

	// ln(y) = 2 * atanh(u) = 2 * (u + u^3/3 + u^5/5 + ...), u = (y-1)/(y+1).
	num, _ := wc.Sub(y, one)
	den, _ := wc.Add(y, one)
	u, _ := wc.Divide(num, den)
	u2, _ := wc.Mul(u, u)
	sum := u
	term := u
	for k := 3; k < 100000; k += 2 {
		term, _ = wc.Mul(term, u2)
		t, _ := wc.Divide(term, New(int64(k), 0))
		sum, _ = wc.Add(sum, t)
		if negligible(t, small) {
			break
		}
	}
	res, _ := wc.Mul(sum, two) // ln(y)
	for i := 0; i < m; i++ {   // undo the sqrt reduction
		res, _ = wc.Mul(res, two)
	}
	return res
}

// Ln returns the natural logarithm of x, rounded to the context.
func (c Context) Ln(x Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, x); ok {
		return r, cond
	}
	if c.mathContextInvalid() {
		return makeNaN(false, nil, false), InvalidContext
	}
	if x.isInf() {
		if x.neg {
			return makeNaN(false, nil, false), InvalidOperation
		}
		return infinity(false), 0
	}
	if x.isZero() {
		return infinity(true), 0 // ln(0) = -Infinity (no signal)
	}
	if x.sign() {
		return makeNaN(false, nil, false), InvalidOperation // ln of a negative
	}
	one := New(1, 0)
	if cmpDecimal(x, one) == 0 {
		return New(0, 0), 0 // ln(1) = 0 exactly
	}
	prec := int(c.Precision)
	if prec <= 0 {
		prec = 34
	}
	wc := workContext(prec, 26)
	return c.finishTranscendental(lnWork(wc, x, -(prec + 20)))
}

// Log10 returns the base-10 logarithm of x, rounded to the context.
func (c Context) Log10(x Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, x); ok {
		return r, cond
	}
	if c.mathContextInvalid() {
		return makeNaN(false, nil, false), InvalidContext
	}
	if x.isInf() {
		if x.neg {
			return makeNaN(false, nil, false), InvalidOperation
		}
		return infinity(false), 0
	}
	if x.isZero() {
		return infinity(true), 0 // log10(0) = -Infinity
	}
	if x.sign() {
		return makeNaN(false, nil, false), InvalidOperation
	}
	// Exact powers of ten (coefficient is 1 followed by zeros) give an integer.
	mag := new(big.Int).Abs(&x.coeff)
	nd := numDigits(mag)
	if mag.Cmp(pow10(uint(nd-1))) == 0 {
		return c.RoundToContext(New(int64(x.exp)+int64(nd)-1, 0))
	}
	prec := int(c.Precision)
	if prec <= 0 {
		prec = 34
	}
	wc := workContext(prec, 28)
	lnx := lnWork(wc, x, -(prec + 22))
	ln10 := lnWork(wc, New(10, 0), -(prec + 22))
	q, _ := wc.Divide(lnx, ln10)
	return c.finishTranscendental(q)
}

// oddInteger reports whether y is an odd integer.
func oddInteger(y Decimal) bool {
	v, ok := y.integerValue()
	return ok && v&1 == 1
}

// isInteger reports whether y is a finite integer, including values too large to
// fit an int64 (a non-negative exponent always denotes a whole number).
func isInteger(y Decimal) bool {
	if y.form != finite {
		return false
	}
	if y.coeff.Sign() == 0 || y.exp >= 0 {
		return true
	}
	if int64(-y.exp) >= int64(numDigits(&y.coeff)) {
		return false // more fractional places than digits
	}
	r := new(big.Int)
	new(big.Int).QuoRem(new(big.Int).Abs(&y.coeff), pow10(uint(-y.exp)), r)
	return r.Sign() == 0
}

// Power returns x**y rounded to the context.
func (c Context) Power(x, y Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, x, y); ok {
		return r, cond
	}
	if c.mathContextInvalid() {
		return makeNaN(false, nil, false), InvalidContext
	}
	one := New(1, 0)
	switch {
	case x.isZero() && y.isZero(): // 0**0 is undefined
		return makeNaN(false, nil, false), InvalidOperation
	case y.isZero(): // x**0 = 1
		return one, 0
	case x.sign() && !x.isZero() && !isInteger(y): // negative base, non-integer exponent
		return makeNaN(false, nil, false), InvalidOperation
	}

	// For two finite non-zero operands, power restricts the adjusted exponent of
	// the exponent (always) and the base (only on the transcendental path) to a
	// fixed range; beyond it the operation raises Invalid_operation. The bounds
	// derive from decNumber's DEC_MAX_MATH and are asymmetric because ln(x) grows.
	if c.MaxExponent != 0 && x.IsFinite() && !x.isZero() && y.IsFinite() {
		const maxMath = 999999
		if adj := adjExp(y); adj > maxMath || adj < 1-2*maxMath {
			return makeNaN(false, nil, false), InvalidOperation
		}
		if !isInteger(y) {
			if adj := adjExp(x); adj > maxMath || adj < 1-2*maxMath {
				return makeNaN(false, nil, false), InvalidOperation
			}
		}
	}

	switch {
	case !x.isInf() && cmpDecimal(x, one) == 0: // x = +1
		if isInteger(y) {
			return one, 0 // 1**integer = 1 exactly
		}
		// 1**(non-integer or infinite) is 1, but inexact and padded to precision.
		p := int(c.Precision)
		if p <= 0 {
			p = 34
		}
		return newDec(pow10(uint(p-1)), int32(-(p - 1))), Inexact | Rounded
	case y.isInf():
		cmp := cmpDecimal(x.CopyAbs(), one)
		if cmp == 0 {
			return one, 0 // (+-1)**inf = 1
		}
		if (cmp > 0) != y.neg { // |x|>1 with +inf, or |x|<1 with -inf
			return infinity(false), 0
		}
		return New(0, 0), 0
	case x.isInf():
		neg := x.neg && oddInteger(y)
		if !y.sign() { // y > 0
			return infinity(neg), 0
		}
		return zeroExp(neg, 0), 0 // y < 0
	case x.isZero():
		neg := x.sign() && oddInteger(y)
		if y.sign() { // 0**negative = +/-Infinity
			return infinity(neg), 0
		}
		return zeroExp(neg, 0), 0 // 0**positive
	}

	// Finite, non-zero x and finite, non-zero y.
	if yi, isInt := y.integerValue(); isInt {
		res, cond := c.intPower(x, yi)
		// A positive integer exponent past DEC_MAX_EMAX is "out of range": it can
		// only be evaluated when the result over- or underflows the context; an
		// in-range result would need an uncomputable exponent, so it is invalid.
		if c.MaxExponent != 0 && yi > 999999999 && cond&(Overflow|Underflow) == 0 {
			return makeNaN(false, nil, false), InvalidContext
		}
		return res, cond
	}
	if x.sign() { // a negative base to a non-integer power is undefined
		return makeNaN(false, nil, false), InvalidOperation
	}
	// x**y = exp(y * ln(x))
	prec := int(c.Precision)
	if prec <= 0 {
		prec = 34
	}
	wc := workContext(prec, 30)
	lnx := lnWork(wc, x, -(prec + 24))
	prod, _ := wc.Mul(y, lnx)
	ewc := wc
	// A tiny product makes x**y extremely close to 1; widen the working precision by
	// its magnitude so the deviation from 1 survives for a correctly-rounded result
	// (needed under directional rounding).
	if a := adjExp(prod); a < 0 {
		ewc = workContext(prec, 30-int(a))
	}
	res, _ := ewc.Exp(prod)
	return c.finishTranscendental(res)
}

// intPower computes x**n for an integer n via repeated squaring at the working
// context, then rounds to the target context.
func (c Context) intPower(x Decimal, n int64) (Decimal, Condition) {
	prec := int(c.Precision)
	if prec <= 0 {
		prec = 34
	}
	wc := workContext(prec, 20)
	neg := n < 0
	if neg {
		n = -n
	}
	result := New(1, 0)
	base := x
	for n > 0 {
		if n&1 == 1 {
			result, _ = wc.Mul(result, base)
		}
		if n >>= 1; n > 0 {
			base, _ = wc.Mul(base, base)
		}
	}
	if neg {
		result, _ = wc.Divide(New(1, 0), result)
	}
	return c.RoundToContext(result)
}
