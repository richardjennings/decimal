package decimal

// RoundingMode selects how a value is rounded when digits are discarded.
// Names and semantics follow the General Decimal Arithmetic specification.
type RoundingMode uint8

const (
	RoundHalfEven RoundingMode = iota // to nearest, ties to even (default)
	RoundHalfUp                       // to nearest, ties away from zero
	RoundHalfDown                     // to nearest, ties toward zero
	RoundDown                         // toward zero (truncate)
	RoundUp                           // away from zero
	RoundCeiling                      // toward +Infinity
	RoundFloor                        // toward -Infinity
	Round05Up                         // toward zero unless last kept digit is 0 or 5
)

func (m RoundingMode) String() string {
	switch m {
	case RoundHalfEven:
		return "half_even"
	case RoundHalfUp:
		return "half_up"
	case RoundHalfDown:
		return "half_down"
	case RoundDown:
		return "down"
	case RoundUp:
		return "up"
	case RoundCeiling:
		return "ceiling"
	case RoundFloor:
		return "floor"
	case Round05Up:
		return "05up"
	default:
		return "unknown"
	}
}

// Condition is a bit set of the exceptional conditions (GDA signals) raised by
// an operation. The finite core raises Rounded and Inexact; the rest are
// reserved for full conformance.
type Condition uint32

const (
	Rounded Condition = 1 << iota
	Inexact
	Overflow
	Underflow
	Subnormal
	Clamped
	DivisionByZero
	DivisionImpossible
	DivisionUndefined
	InvalidOperation
	ConversionSyntax
	InvalidContext // context too extreme for a mathematical function (exp/ln/log10/power)
)

// Has reports whether every condition in f is set in c.
func (c Condition) Has(f Condition) bool { return c&f == f }

// Context fixes the working precision and rounding mode for an operation.
// A Precision of 0 means unlimited: results are exact and never rounded.
type Context struct {
	Precision   int32
	Rounding    RoundingMode
	MaxExponent int32 // Emax (only enforced when Bounded)
	MinExponent int32 // Emin (only enforced when Bounded)
	Clamp       bool  // fold exponents down to Etop (decimal64/128 behaviour)
	Bounded     bool  // enforce exponent limits (overflow / underflow / subnormal / clamp)
}

// Exact never rounds; Add/Sub/Mul are exact for finite operands.
var Exact = Context{Precision: 0, Rounding: RoundHalfEven}

// Basic mirrors the GDA suite's default starting context.
var Basic = Context{Precision: 9, Rounding: RoundHalfEven}
