package store

// This file owns the store-side secret exclusion that elevates PROF-03 (D-07–D-10):
// on Commit, a LITERAL secret is replaced by a resolver-agnostic SecretRef, its value
// captured into the injected keychain/vault backend, and the literal is cleared so it
// never enters the committed tree; an already-DYNAMIC secret commits verbatim (D-08).
//
// It reuses the ALREADY-CLASSIFIED Entry verdict — there is NO new secret regex and NO
// new AST walk here. The classifier (core/shell/zsh/classify.go's secretRe) set
// Entry.Category == CatSecrets at parse/build time, and the parser set Entry.Dynamic;
// excludeSecrets reads only those populated fields, so core/store stays shell-agnostic
// (it never imports core/shell/zsh). The SecretRef.Kind is stamped from the ACTIVE
// driver's kc.Kind(), not a hardcoded constant, so a machine without a system keychain
// produces a `file`-kind reference that Ph4/5 dereferences from the vault (T-03-09).

import (
	"context"
	"fmt"

	"zsh-pro/core/model"
)

// excludeSecrets rewrites p so no literal secret value survives into the committed
// tree, capturing each into the injected backend and reporting what was withheld.
//
// It operates on a DEFENSIVE COPY of the profile (a fresh Entries slice): the caller's
// original Profile is never mutated, so the Phase 2 round-trip comparison and any
// caller-held reference see the literal value unchanged. The returned Profile is the
// one Commit marshals + commits.
//
// The D-08 predicate uses ONLY already-populated Entry fields (the classifier's
// CatSecrets verdict + the parser's Dynamic flag — no new inspection):
//   - LITERAL  (Category==CatSecrets && !Dynamic && Value != ""): capture Value into the
//     backend under key Names[0], replace the entry with a SecretRef{Kind: kc.Kind()},
//     clear Value, and append a WithheldSecret. A literal secret with a nil backend is
//     ErrSecretBackendUnavailable (never a nil-panic); a backend Store failure aborts
//     the whole Commit so a half-excluded profile is never committed.
//   - DYNAMIC  (Category==CatSecrets && Dynamic): already a late-bound pointer — committed
//     verbatim, not captured, not reported (D-08).
//   - everything else: passed through unchanged.
//
// No secret value is ever logged or echoed (Pitfall 3 / ASVS V7). The ctx is accepted to
// match Commit's call shape and leave room for a context-aware backend without a future
// signature change; the current backends are synchronous.
func excludeSecrets(_ context.Context, p model.Profile, kc KeychainDriver) (model.Profile, WithheldReport, error) {
	// Defensive copy of the entry slice so the caller's Profile is untouched. A nil
	// Entries stays nil (no allocation) — the secret-free / empty-profile path is a
	// pure no-op that returns the profile and a nil report, preserving the Phase 2
	// round-trip equality.
	if len(p.Entries) == 0 {
		return p, nil, nil
	}
	entries := make([]model.Entry, len(p.Entries))
	copy(entries, p.Entries)

	var report WithheldReport
	for i := range entries {
		e := &entries[i]
		if !isLiteralSecret(*e) {
			// Not a literal secret: a dynamic secret (already a pointer, D-08) or any
			// non-secret entry is committed verbatim.
			continue
		}

		key := e.Names[0] // isLiteralSecret guarantees a non-empty Names.
		if kc == nil {
			// A literal secret with no backend cannot be safely excluded: refuse rather
			// than commit the literal or nil-panic. The composition root always injects a
			// non-nil driver (NewOSKeychainDriver falls back to the vault), so this guards
			// only test-only / future misuse construction.
			return model.Profile{}, nil, ErrSecretBackendUnavailable
		}
		if err := kc.Store(key, e.Value); err != nil {
			// Capture failed: abort the whole Commit. Never fall through and commit the
			// literal — that would leak the secret into the tree.
			return model.Profile{}, nil, err
		}

		// Replace the literal with a reference stamped with the ACTIVE backend's kind
		// (keychain when security/secret-tool is present, file for the vault fallback —
		// T-03-09). The Secret pointer is the AUTHORITATIVE record (it serializes as
		// {"kind":..,"key":..} in profile.json and is what Ph4/5 dereferences).
		ref := &model.SecretRef{Kind: kc.Kind(), Key: key}
		e.Secret = ref

		// The literal must leave BOTH derived text fields, not just Value: Entry.Text is
		// the verbatim source (serialized as the JSON "text" field AND the Regenerator's
		// fallback for an empty Value), and Entry.Value feeds the Regenerator's emitted
		// `export NAME=<value>` line for profile.zsh. Clearing only Value would (a) leave
		// the literal in profile.json's "text" field and (b) make Regenerate fall back to
		// the literal-bearing Text — so the secret would still enter the committed tree
		// (T-03-03). Replace both with an inert, single-quoted reference placeholder
		// carrying the kind:key, so neither committed blob holds the literal while the
		// profile.zsh view stays human-meaningful. This is store-side DATA substitution
		// (a placeholder string), NOT zsh codegen — core/store stays shell-agnostic; the
		// real deref/emit syntax is Ph4/5's job (the placeholder is never sourced in Ph3).
		placeholder := secretRefValue(*ref)
		if e.Exported {
			e.Text = fmt.Sprintf("export %s=%s", key, placeholder)
		} else {
			e.Text = fmt.Sprintf("%s=%s", key, placeholder)
		}
		e.Value = placeholder

		report = append(report, WithheldSecret{Name: key, StartLine: e.StartLine})
	}

	return model.Profile{Entries: entries}, report, nil
}

// secretRefValue renders the inert placeholder that stands in for an excluded
// literal's value in both Entry.Text and Entry.Value. It is single-quoted so it is a
// zsh-inert literal (no expansion, no command substitution) if the profile.zsh view is
// ever sourced, and it embeds the resolver-agnostic kind:key (D-09) so the committed
// derived view documents WHAT was withheld and from WHICH backend. It is NOT the deref
// expression — Ph4/5 owns reading the value back from the backend and emitting the real
// assignment; in Phase 3 this placeholder only guarantees the literal never reaches the
// tree while keeping profile.zsh human-readable. The leading `<` makes it visibly a
// non-value sentinel rather than a plausible secret.
func secretRefValue(ref model.SecretRef) string {
	return fmt.Sprintf("'<zsh-pro secret %s:%s>'", ref.Kind, ref.Key)
}

// isLiteralSecret is the D-08 LITERAL predicate over already-populated Entry fields:
// a secret-named var (the classifier's CatSecrets verdict) whose value is a static
// literal (not Dynamic) and non-empty. An already-dynamic secret (export TOKEN=$(...))
// is NOT literal — it is a late-bound pointer committed verbatim. Names must be
// non-empty so Names[0] is a valid SecretRef key (a CatSecrets assignment always has a
// name; the guard makes the precondition explicit rather than risking an index panic).
func isLiteralSecret(e model.Entry) bool {
	return e.Category == model.CatSecrets && !e.Dynamic && e.Value != "" && len(e.Names) > 0
}
