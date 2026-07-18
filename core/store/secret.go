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
//
// FAIL-CLOSED (T-03-03, CR-01/CR-02): exclusion can only reason faithfully about a
// secret that is a SINGLE-NAME SCALAR assignment, because the parser COLLAPSES a
// multi-name assignment (`export A=$HOME B=secret`) into one Entry whose Value holds
// only the LAST segment and whose Dynamic flag is OR'd across ALL segments — so the
// literal can hide in Entry.Text under a name that is not Names[0], and the Dynamic
// guard can be tripped by an unrelated segment. An array secret (`export ARR=(...)`)
// likewise carries its literal only in Text with an empty Value. For any CatSecrets
// entry whose shape is not a single-name scalar (isExcludableSecretShape false, or a
// single-name assignment with no scalar literal), exclusion CANNOT prove the literal
// is captured, so it ABORTS the Commit with ErrUnsafeSecretShape rather than fall
// through to a verbatim commit that would leak the literal into the tree.

import (
	"context"
	"fmt"

	"zsh-pro/core/model"
)

// excludeSecrets rewrites p so no literal secret value survives into the committed
// tree, capturing each into the injected backend and reporting what was withheld.
//
// It operates on a fresh COPY of the Entries slice (a shallow copy of the entry
// values): the caller's original Profile is never mutated, so the Phase 2 round-trip
// comparison and any caller-held reference see the literal value unchanged. The copy
// is shallow — a copied Entry shares the caller's Names backing array and Secret
// pointer — but the loop only ever REPLACES value-typed fields (Text, Value,
// ValueMode), CLEARS RuntimeValue, and INSTALLS a new Secret pointer; it never writes
// through e.Names[i], *e.RuntimeValue, or *e.Secret, so no aliased state of the caller
// is mutated in place. (Names is additionally cloned
// at the IR boundary in ir.Build, so even the shared backing array is not the
// parser's.) The returned Profile is the one Commit marshals + commits.
//
// The verdict over each entry uses ONLY already-populated Entry fields (the
// classifier's CatSecrets verdict + the parser's Dynamic flag — no new inspection),
// and FAILS CLOSED on any CatSecrets shape it cannot model faithfully:
//   - NOT a CatSecrets entry: passed through unchanged.
//   - CatSecrets but NOT an excludable shape (isExcludableSecretShape false — a
//     multi-name assignment, a non-assignment secret, CR-01/CR-02): ABORT the Commit
//     with ErrUnsafeSecretShape. The literal can hide in Text under a name other than
//     Names[0], so the only safe action is to refuse rather than leak (T-03-03).
//   - excludable-shape DYNAMIC (single-name, Dynamic): already a late-bound pointer —
//     committed verbatim, not captured, not reported (D-08). This is the legitimate
//     `export TOKEN=$(vault get)` case.
//   - excludable-shape LITERAL (single-name, !Dynamic, Value != ""): capture Value into
//     the backend under key Names[0], replace the entry with a SecretRef{Kind:
//     kc.Kind()}, clear Value+Text, and append a WithheldSecret. A literal secret with a
//     nil backend is ErrSecretBackendUnavailable (never a nil-panic); a backend Store
//     failure aborts the whole Commit so a half-excluded profile is never committed.
//   - excludable-shape but NO scalar literal (single-name, !Dynamic, Value == ""): a
//     degenerate or array secret (`export ARR=(sk-one sk-two)` leaves Value empty and
//     hides its literal in Text). The store cannot model that from Entry fields, so it
//     ABORTS with ErrUnsafeSecretShape rather than commit the verbatim Text (T-03-03).
//
// No secret value is ever logged or echoed (Pitfall 3 / ASVS V7). The ctx is accepted to
// match Commit's call shape and leave room for a context-aware backend without a future
// signature change; the current backends are synchronous.
func excludeSecrets(_ context.Context, p model.Profile, kc KeychainDriver) (model.Profile, WithheldReport, error) {
	// Shallow copy of the entry slice so the caller's Profile is untouched. A nil
	// Entries stays nil (no allocation) — the secret-free / empty-profile path is a
	// pure no-op that returns the profile and a nil report, preserving the Phase 2
	// round-trip equality. The copy shares each Entry's Names backing array and Secret
	// pointers with the caller, but the loop below only replaces value-typed fields,
	// clears RuntimeValue, and installs a new Secret pointer (never writes through the
	// shared array/pointers), so the caller is never corrupted
	// (TestExcludeSecretsDefensiveCopy pins this).
	if len(p.Entries) == 0 {
		return p, nil, nil
	}
	entries := make([]model.Entry, len(p.Entries))
	copy(entries, p.Entries)

	var report WithheldReport
	for i := range entries {
		e := &entries[i]
		if e.Category != model.CatSecrets {
			// Not a secret-named var: committed verbatim, untouched.
			continue
		}
		if !isExcludableSecretShape(*e) {
			// A CatSecrets entry whose shape does not faithfully model a single secret
			// segment (a multi-name assignment whose collapsed Value/Dynamic cannot tell
			// us WHICH segment is the secret, or a non-assignment secret). Excluding it
			// would risk leaving the literal in Text (CR-01) or capturing it under the
			// wrong key (CR-02). Fail closed (T-03-03): never fall through to a verbatim
			// commit that would leak the literal.
			return model.Profile{}, nil, ErrUnsafeSecretShape
		}
		if e.Dynamic {
			// Single-name dynamic secret (export TOKEN=$(...)): already a late-bound
			// pointer, committed verbatim and not reported (D-08).
			continue
		}
		if e.Value == "" {
			// Single-name, non-dynamic, but no scalar literal in Value: an array secret
			// (`export ARR=(sk-one sk-two)`) keeps its literal only in Text, or a
			// degenerate empty assignment. The store cannot prove from Entry fields that
			// Text holds no literal, so fail closed rather than commit Text verbatim.
			return model.Profile{}, nil, ErrUnsafeSecretShape
		}

		key := e.Names[0] // isExcludableSecretShape guarantees exactly one name.
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
		// RuntimeValue carries the decoded literal and therefore must be redacted at
		// the same boundary as Text and Value. Marking the entry Unsupported prevents
		// activation from falling back to the inert placeholder; SecretRef is now the
		// authoritative source until the runtime secret resolver supplies the value.
		e.RuntimeValue = nil
		e.ValueMode = model.ValueModeUnsupported

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

// isExcludableSecretShape reports whether a CatSecrets entry is the ONLY shape whose
// Value/Dynamic faithfully describe its single secret segment: a single-name scalar
// assignment. The parser collapses a multi-name assignment (`export A=$HOME B=secret`)
// into ONE Entry with Names=[A,B], a Value that is only the LAST segment, and a Dynamic
// flag OR'd across ALL segments — so for anything but len(Names)==1 the literal may live
// in Text under a name other than Names[0] and the per-entry Value/Dynamic cannot be
// trusted (CR-01/CR-02). A non-assignment secret (Kind != KindAssignment) is likewise
// not a clean NAME=value to capture. excludeSecrets fails closed (ErrUnsafeSecretShape)
// on every CatSecrets entry that is NOT this shape rather than risk leaking the literal.
//
// This is a SHAPE gate only: it does not decide literal-vs-dynamic. Within an excludable
// shape, excludeSecrets still splits Dynamic (committed verbatim, D-08) from a non-empty
// scalar literal (captured + referenced), and fails closed on a single-name secret with
// no scalar literal (an array `export ARR=(...)` hides its literal in Text with an empty
// Value).
func isExcludableSecretShape(e model.Entry) bool {
	return e.Category == model.CatSecrets &&
		e.Kind == model.KindAssignment &&
		len(e.Names) == 1
}
