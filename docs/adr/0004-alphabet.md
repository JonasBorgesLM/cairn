# ADR-0004: base62 by default, an unambiguous alphabet available, with the entropy floor preserved

## Status
Accepted

## Context
base62 (`0-9A-Za-z`) is the standard choice. It contains six pairs that humans
confuse when transcribing: `0`/`O`/`o` and `1`/`l`/`I`. A link read off a poster,
dictated over a phone, or retyped from a printed invoice will be got wrong, and
the failure is silent — the visitor lands on a 404 and blames the service.

Removing them costs keyspace. The question is how much, and whether it matters.

## Decision
Two alphabets ship, `AlphabetBase62` is the default, and **the entropy floor is
preserved by adjusting the length rather than by accepting a weaker keyspace.**

| Alphabet | Size | Bits/rune | Default length | Total |
| --- | --- | --- | --- | --- |
| `AlphabetBase62` | 62 | 5.954 | 10 | 59.5 bits |
| `AlphabetUnambiguous` | 56 | 5.807 | 11 | 63.9 bits |

`AlphabetUnambiguous` drops `0 O o 1 l I`. The per-rune cost is **0.147 bits** —
about 2.5% — which is nothing. Selecting it therefore raises the default length
to 11 so that the resulting keyspace is at least the base62 default's, and the
security parameter of ADR-0002 is never quietly traded for legibility. One extra
character is the entire price of removing a whole class of human error.

base62 stays the default because the common case is a link that is clicked or
pasted, never transcribed, and shorter is genuinely better there. Consumers whose
links are printed, spoken or shown in a QR fallback string should choose the
other one, and the documentation says which situation is which rather than
leaving it to taste.

`NewAlphabet` accepts a custom set and rejects duplicates, non-ASCII runes, and
anything outside `[A-Za-z0-9_-]` — the URL-path-safe set. A rune requiring
percent-encoding in a path segment would make the code's own rendering ambiguous,
which defeats SR-21's premise that a code is validatable before it becomes a key.

## Consequences
- `WithAlphabet(AlphabetUnambiguous)` changes the default length. This is
  surprising if undocumented, so it is stated at both option sites, and an
  explicit `WithCodeLength` still wins — with the density check of ADR-0002
  applied to whatever the consumer chose.
- Case sensitivity is retained in both alphabets. Case-insensitive codes would
  cost 0.79 bits/rune (62 → 36) and, more importantly, require case folding on
  the resolve path *before* key construction, which adds a step between
  untrusted input and the store that SR-21 would then have to cover too. Not
  worth it; a consumer who wants it can supply a 36-rune alphabet.
- The two alphabets are not interchangeable for an existing dataset: codes issued
  under one may be invalid under the other, and SR-21 rejects them at resolve.
  Changing the alphabet of a live deployment is a migration, not a config edit,
  and the documentation says so.
