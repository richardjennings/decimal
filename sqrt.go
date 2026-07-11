package decimal

import "math/big"

func floorDiv2(x int64) int64 {
	if x >= 0 {
		return x / 2
	}
	return (x - 1) / 2
}

// Sqrt returns the square root of a, correctly rounded to the context.
func (c Context) Sqrt(a Decimal) (Decimal, Condition) {
	if r, cond, ok := nanResult(c.Precision, a); ok {
		return r, cond
	}
	if a.isZero() {
		return c.finalize(zeroExp(a.sign(), floorDiv2(int64(a.exp))), 0)
	}
	if a.sign() { // negative finite or -Infinity
		return makeNaN(false, nil, false), InvalidOperation
	}
	if a.isInf() {
		return infinity(false), 0
	}

	prec := c.Precision
	if prec <= 0 {
		prec = 34
	}
	ca := new(big.Int).Abs(&a.coeff)
	e := int64(a.exp)
	if e&1 != 0 { // work with an even exponent so e/2 is exact
		ca.Mul(ca, big.NewInt(10))
		e--
	}
	// Scale up until the coefficient has enough digits for prec+1 root digits.
	for numDigits(ca) < 2*(int(prec)+1) {
		ca.Mul(ca, big.NewInt(100))
		e -= 2
	}
	q := new(big.Int).Sqrt(ca)
	rem := new(big.Int).Sub(ca, new(big.Int).Mul(q, q)) // sqrt remainder, >= 0

	// Round q down to prec significant digits, using rem as a sticky bit.
	drop := numDigits(q) - int(prec)
	if drop < 0 {
		drop = 0
	}
	divisor := pow10(uint(drop))
	qHi := new(big.Int)
	r2 := new(big.Int)
	qHi.QuoRem(q, divisor, r2)
	exp := e/2 + int64(drop)

	if r2.Sign() != 0 || rem.Sign() != 0 {
		// Inexact. A subnormal result would double-round (once here, once in
		// finalize), so route it through roundFinite for a single rounding —
		// q carries >= 2 guard digits there, so a round-to-odd sticky is safe.
		if etiny := int64(c.MinExponent) - int64(c.Precision) + 1; c.Bounded && exp < etiny {
			if rem.Sign() != 0 && q.Bit(0) == 0 {
				q.Add(q, big.NewInt(1))
			}
			return c.roundFinite(q, e/2, 0)
		}
		cond := Inexact | Rounded
		switch half := new(big.Int).Rsh(divisor, 1); r2.Cmp(half) {
		case 1:
			qHi.Add(qHi, big.NewInt(1))
		case 0:
			if rem.Sign() != 0 || qHi.Bit(0) == 1 { // above the half, or ties-to-even
				qHi.Add(qHi, big.NewInt(1))
			}
		}
		if int32(numDigits(qHi)) > prec { // rounding carry
			qHi.Quo(qHi, big.NewInt(10))
			exp++
		}
		return c.finalize(newDec(qHi, clampExp(exp)), cond)
	}

	// Exact perfect square: strip trailing zeros toward the ideal exponent.
	ideal := floorDiv2(int64(a.exp))
	ten := big.NewInt(10)
	quo := new(big.Int)
	mod := new(big.Int)
	for exp < ideal {
		quo.QuoRem(qHi, ten, mod)
		if mod.Sign() != 0 {
			break
		}
		qHi.Set(quo)
		exp++
	}
	var cond Condition
	if exp > ideal { // coefficient shortened past the ideal exponent
		cond = Rounded
	}
	return c.finalize(newDec(qHi, clampExp(exp)), cond)
}
