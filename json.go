package decimal

import "encoding/json"

// MarshalJSON encodes the decimal as a JSON string in its canonical form, e.g.
// "1.50". A string is used rather than a JSON number so precision and trailing
// zeros survive the round-trip exactly.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON decodes a decimal from a JSON string (the form MarshalJSON emits) or
// a bare JSON number.
func (d *Decimal) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		s = string(b) // fall back to a bare JSON number
	}
	v, err := NewFromString(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}
