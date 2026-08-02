package model

// WithheldSecret identifies a source declaration whose literal value was
// withheld. It deliberately carries no secret value or backend details.
type WithheldSecret struct {
	Name      string
	StartLine int
}

// WithheldReport is the value-free, source-ordered result of secret exclusion.
type WithheldReport []WithheldSecret
