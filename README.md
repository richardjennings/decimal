# decimal

[![CI](https://github.com/richardjennings/decimal/actions/workflows/ci.yml/badge.svg)](https://github.com/richardjennings/decimal/actions/workflows/ci.yml)

An arbitrary-precision decimal arithmetic library in Go, conformant with the
[General Decimal Arithmetic](https://speleotrove.com/decimal/) (GDA) specification
— the basis for IEEE 754-2008 decimal, Python's `decimal`, and Java's `BigDecimal`.

A finite value is `coefficient × 10^exp`, with the coefficient held as a
`math/big.Int` and a base-ten exponent, so decimal fractions like `0.1` are exact.

## Status

**Passes the entire IBM General Decimal Arithmetic test suite: 65,127 cases, 0
failures, 0 skips, across 144 files.** Every GDA operation is implemented, along
with the `decimal32/64/128` densely-packed-decimal interchange encodings.

## Install

```sh
go get github.com/richardjennings/decimal
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/richardjennings/decimal"
)

func main() {
	// 0.1 + 0.2 is exact — no binary-float surprises.
	c := decimal.Basic // precision 9, round half-even
	sum, _ := c.Add(decimal.MustParse("0.1"), decimal.MustParse("0.2"))
	fmt.Println(sum) // 0.3

	// Round to a context and inspect the signals raised.
	ctx := decimal.Context{Precision: 5, Rounding: decimal.RoundHalfEven}
	q, cond := ctx.Mul(decimal.MustParse("1.2345"), decimal.MustParse("6.789"))
	fmt.Println(q, cond.Has(decimal.Inexact)) // 8.3810 true

	// IEEE 754 decimal64 interchange encoding.
	enc := decimal.MustParse("-7.50").Encode(8) // 8 bytes (decimal64)
	fmt.Printf("%X\n", enc)                      // A2300000000003D0
	fmt.Println(decimal.Decode(enc))             // -7.50
}
```

Operations are methods on `Context`, which fixes the precision, rounding mode,
and (optionally) the exponent limits. Each returns the result together with a
`Condition` bit-set of the GDA signals (`Inexact`, `Rounded`, `Overflow`,
`Subnormal`, …). Use the zero-limit `decimal.Exact` context for unrounded
results, or set `MaxExponent` / `MinExponent` / `Clamp` / `Bounded` to model a
fixed format.

## Operations

- **Arithmetic** — `Add` `Sub` `Mul` `Divide` `DivideInt` `Rem` `RemNear` `FMA`
  `Plus` `Minus` `Abs`, all eight rounding modes, `NaN`/`sNaN`/`±Infinity`,
  signed zero, and NaN payloads.
- **Compare / select** — `Compare` `CompareTotal` `CompareTotMag` `CompareSig`
  `Max` `Min` `MaxMag` `MinMag` `SameQuantum`, `Copy` `CopyAbs` `CopySign` `Neg`.
- **Rounding / exponent** — `Quantize` `Rescale` `ToIntegral` `RoundToContext`
  `NextPlus` `NextMinus` `NextToward` `Reduce` `LogB` `ScaleB` `Trim`
  `Canonical` `Class`.
- **Transcendental** — `Sqrt` `Exp` `Ln` `Log10` `Power`, correctly rounded.
- **Logical / shift** — `And` `Or` `Xor` `Invert` `Shift` `Rotate`.
- **Encoding** — `Encode(width)` / `Decode(bytes)` for decimal32/64/128 (DPD).
- **Strings** — `String` (to-scientific) and `StringEng` (to-engineering).

## Design notes

- **Single rounding at `Etiny`.** A `roundFinite` step folds precision rounding
  and the subnormal floor together, so gradual underflow never double-rounds.
- **Precision-bounded alignment.** `Add` and `Compare` keep only the digits that
  can affect a rounded result, carrying a round-to-odd sticky bit — so operands
  whose exponents span billions never materialise `10^span`.
- **Correctly-rounded transcendentals.** `exp`/`ln`/`log10`/`power` evaluate at
  guard precision with argument reduction; `power` widens the working precision
  adaptively when the result sits within a ULP of 1, so directional rounding
  stays correct.
- **DPD interchange codec.** `encoding.go` implements densely-packed-decimal
  encode/decode for all three interchange widths, including non-canonical inputs.

## Layout

| File | Purpose |
|------|---------|
| `decimal.go` | `Decimal` type, parsing, core finite arithmetic |
| `boundedadd.go` | precision-bounded alignment for `Add`/`Compare` |
| `round.go`, `finalize.go` | rounding engine and the exponent-limit model |
| `divide.go`, `sqrt.go` | division family and square root |
| `compare.go`, `misc.go`, `next.go` | comparison, logical/shift, next-toward |
| `transcend.go` | `exp`, `ln`, `log10`, `power` |
| `encoding.go` | decimal32/64/128 DPD interchange codec |
| `format.go`, `special.go`, `context.go` | strings, `NaN`/`Inf`, `Context`/`Condition` |
| `dectest/` | parser and runner for the IBM `.decTest` conformance suite |

## Testing

```sh
make test        # unit tests, codec vectors, fuzz seeds, the sample .decTest
make testdata    # download the full IBM suite (git-ignored, ~4 MB)
make suite       # run the full suite (65,127 cases; expect 0 failed, 0 skipped)
make fuzz        # differential fuzz of Add/Mul against math/big.Rat
make bench       # benchmarks
```

CI (`.github/workflows/ci.yml`) runs `go vet`, race-enabled tests, the full
conformance suite, and a fuzz smoke test on every push.

## Test data

The `.decTest` cases are © Mike Cowlishaw / IBM, under the permissive ICU
licence. `dectest/testdata/sample.decTest` is a small hand-written sample
committed to the repo; the full suite is fetched on demand by `make testdata`
and is git-ignored.
