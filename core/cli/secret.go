package cli

import (
	"errors"

	"zsh-pro/core/model"
)

var (
	errSecretResolverUnavailable = errors.New("secret resolver unavailable")
	errSecretResolution          = errors.New("secret resolution failed")
)

// SecretResolver is the runtime-only subset of the store secret backend. A
// profile records a SecretRef's backend kind, so resolving it here must never
// guess across backends or return the retrieved value in an error.
type SecretResolver interface {
	Retrieve(key string) (string, error)
	Kind() model.SecretRefKind
}

// resolveSecretRefs returns an activation-only profile copy. Persisted profile
// entries stay redacted: only entries with an authoritative SecretRef receive a
// temporary literal RuntimeValue for activate.Build to consume.
func resolveSecretRefs(p model.Profile, resolver SecretResolver) (model.Profile, error) {
	var entries []model.Entry
	for i := range p.Entries {
		ref := p.Entries[i].Secret
		if ref == nil {
			continue
		}
		if resolver == nil {
			return model.Profile{}, errSecretResolverUnavailable
		}
		if ref.Key == "" || ref.Kind != resolver.Kind() {
			return model.Profile{}, errSecretResolution
		}
		value, err := resolver.Retrieve(ref.Key)
		if err != nil {
			return model.Profile{}, errSecretResolution
		}
		if entries == nil {
			entries = make([]model.Entry, len(p.Entries))
			copy(entries, p.Entries)
		}
		entries[i].RuntimeValue = &value
		entries[i].ValueMode = model.ValueModeLiteral
		entries[i].Dynamic = false
	}
	if entries == nil {
		return p, nil
	}
	p.Entries = entries
	return p, nil
}
