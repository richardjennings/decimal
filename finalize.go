package decimal

import "math/big"

// roundFinite rounds a non-zero finite value (coeff × 10^exp) to the context in
// a SINGLE step: it combines precision rounding with the subnormal floor (Etiny)
// so no double-rounding occurs, then applies overflow and clamping. Callers must
// handle an exact-zero coefficient themselves.
func (c Context) roundFinite(coeff *big.Int, exp int64, cond Condition) (Decimal, Condition) {
	nd := int64(numDigits(coeff))
	prec := int64(c.Precision)

	drop := int64(0)
	if prec > 0 && nd > prec {
		drop = nd - prec
	}
	subnormal := false
	if c.Bounded {
		if etiny := int64(c.MinExponent) - prec + 1; exp+drop < etiny {
			drop = etiny - exp
			subnormal = true
		}
	}

	if drop <= 0 {
		return c.finalize(newDec(coeff, clampExp(exp)), cond)
	}
	if drop > nd { // strictly beyond the leading digit: value is < half a ULP
		var d Decimal
		d.coeff.Set(coeff)
		z := c.underflowToZero(d, exp+drop)
		cond |= Rounded | Inexact | Subnormal | Underflow
		if z.coeff.Sign() == 0 {
			cond |= Clamped
		}
		return z, cond
	}

	q, _, k := shift(coeff, int32(drop), c.Rounding)
	cond |= k
	rexp := exp + drop
	if prec > 0 && int64(numDigits(q)) > prec { // rounding carry (e.g. 999… → 1000…)
		q = new(big.Int).Quo(q, big.NewInt(10))
		rexp++
	}
	if subnormal {
		cond |= Subnormal
		if cond&Inexact != 0 {
			cond |= Underflow
		}
		if q.Sign() == 0 { // rounded down to zero: exponent clamped to Etiny
			cond |= Clamped
		}
	}
	res := newDec(q, clampExp(rexp))
	if q.Sign() == 0 {
		res.neg = coeff.Sign() < 0 // preserve the sign of an underflowed zero
	}
	return c.finalize(res, cond)
}

// finalize applies the context's exponent limits to a rounded finite result:
// overflow to +/-Infinity (or the largest finite), subnormal/underflow rounding,
// and, when clamping is enabled, folding the exponent down to Etop.
func (c Context) finalize(d Decimal, cond Condition) (Decimal, Condition) {
	if !c.Bounded || d.form != finite {
		return d, cond
	}
	emax := int64(c.MaxExponent)
	emin := int64(c.MinExponent)
	etiny := emin - int64(c.Precision) + 1
	etop := emax - int64(c.Precision) + 1

	if d.coeff.Sign() == 0 {
		exp := int64(d.exp)
		hi := emax
		if c.Clamp {
			hi = etop
		}
		switch {
		case exp < etiny:
			exp, cond = etiny, cond|Clamped
		case exp > hi:
			exp, cond = hi, cond|Clamped
		}
		z := New(0, int32(exp))
		z.neg = d.neg
		return z, cond
	}

	switch adj := int64(d.exp) + int64(numDigits(&d.coeff)) - 1; {
	case adj > emax:
		return c.overflow(d.sign()), cond | Overflow | Inexact | Rounded

	case adj < emin: // subnormal
		out := cond | Subnormal
		res := d
		if drop := etiny - int64(d.exp); drop > 0 {
			if drop > int64(numDigits(&d.coeff)) {
				// underflows past the smallest subnormal digit: rounds to zero
				// (or the tiniest unit) without materialising pow10(drop)
				res = c.underflowToZero(d, etiny)
				out |= Inexact | Rounded
			} else {
				q, _, k := shift(&d.coeff, int32(drop), c.Rounding)
				res = newDec(q, int32(etiny))
				if q.Sign() == 0 {
					res.neg = d.sign() // preserve the sign of an underflowed zero
				}
				out |= k
			}
		}
		if out&Inexact != 0 {
			out |= Underflow
		}
		if res.coeff.Sign() == 0 { // underflowed to zero: exponent clamped up to Etiny
			out |= Clamped
		}
		return res, out

	default:
		if c.Clamp && int64(d.exp) > etop {
			pad := int64(d.exp) - etop
			coeff := new(big.Int).Mul(new(big.Int).Abs(&d.coeff), pow10(uint(pad)))
			res := newDec(coeff, int32(etop))
			if d.sign() {
				res = res.Neg()
			}
			return res, cond | Clamped
		}
		return d, cond
	}
}

// overflow returns the result of an overflow, which is +/-Infinity for the
// round-to-nearest and round-up modes and the largest finite otherwise.
func (c Context) overflow(neg bool) Decimal {
	toInf := true
	switch c.Rounding {
	case RoundDown, Round05Up:
		toInf = false
	case RoundCeiling:
		toInf = !neg
	case RoundFloor:
		toInf = neg
	}
	if toInf {
		return infinity(neg)
	}
	return c.maxFinite(neg)
}

// maxFinite returns the largest finite value: precision nines × 10^Etop.
func (c Context) maxFinite(neg bool) Decimal {
	coeff := new(big.Int).Sub(pow10(uint(c.Precision)), big.NewInt(1))
	res := newDec(coeff, int32(emaxEtop(c)))
	if neg {
		res = res.Neg()
	}
	return res
}

func emaxEtop(c Context) int64 {
	return int64(c.MaxExponent) - int64(c.Precision) + 1
}

// subnormalFlag returns Subnormal if d is a finite non-zero value whose adjusted
// exponent is below Emin. Used by operations (e.g. quantize) that fix the result
// exponent and so do not underflow-round, but must still flag subnormal results.
func (c Context) subnormalFlag(d Decimal) Condition {
	if c.Bounded && d.form == finite && d.coeff.Sign() != 0 &&
		int64(d.exp)+int64(numDigits(&d.coeff))-1 < int64(c.MinExponent) {
		return Subnormal
	}
	return 0
}

// underflowToZero returns the result of a nonzero value that rounds away below
// the smallest subnormal: zero, or the tiniest unit under round-away modes.
func (c Context) underflowToZero(d Decimal, etiny int64) Decimal {
	up := false
	switch c.Rounding {
	case RoundUp, Round05Up:
		up = true
	case RoundCeiling:
		up = !d.sign()
	case RoundFloor:
		up = d.sign()
	}
	if !up {
		z := New(0, int32(etiny))
		z.neg = d.sign()
		return z
	}
	res := New(1, int32(etiny))
	if d.sign() {
		res = res.Neg()
	}
	return res
}
