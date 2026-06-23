package testgen

import "math/rand"

// Mutator corrupts rendered config bytes to test engine robustness.
type Mutator struct{ rng *rand.Rand }

// NewMutator returns a Mutator driven by rng.
func NewMutator(rng *rand.Rand) *Mutator { return &Mutator{rng: rng} }

// Corrupt applies one random corruption operator to a copy of src and returns
// the result. It never mutates src.
func (m *Mutator) Corrupt(src []byte) []byte {
	out := append([]byte(nil), src...)
	if len(out) == 0 {
		return out
	}
	switch m.rng.Intn(5) {
	case 0: // truncate at a random point
		return out[:m.rng.Intn(len(out))]
	case 1: // overwrite a random byte
		i := m.rng.Intn(len(out))
		out[i] = byte(m.rng.Intn(256))
		return out
	case 2: // insert an unbalanced double quote
		i := m.rng.Intn(len(out))
		return append(out[:i:i], append([]byte{'"'}, out[i:]...)...)
	case 3: // inject an invalid UTF-8 byte
		i := m.rng.Intn(len(out))
		return append(out[:i:i], append([]byte{0xff}, out[i:]...)...)
	default: // drop the first closing brace if present, else append a stray '{'
		for i, c := range out {
			if c == '}' {
				return append(out[:i], out[i+1:]...)
			}
		}
		return append(out, '{')
	}
}
