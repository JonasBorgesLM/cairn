<!--
Title must be a Conventional Commit -- it becomes the subject on a squash merge:
  feat(policy): reject IPv4-mapped IPv6 destinations
-->

## What and why

<!-- The diff says what changed. Say why it needed to. -->

## Traceability

- Requirements: <!-- SR-18, FR-02 -- or "none" with a reason -->
- Decisions: <!-- ADR-0008 -- or "none" -->
- Closes: <!-- #33 -->

## Security checklist

Delete the lines that do not apply; do not delete the ones that do.

- [ ] This touches an `SR-` requirement, and its test has been **seen to fail**
      with the protection removed. The negative control is noted in a comment
      above the test.
- [ ] No new dependency in the core module (ADR-0001).
- [ ] No new formatting path on `Destination` that could emit an unredacted URL
      (SR-15).
- [ ] Code validation still precedes key construction (SR-21).
- [ ] `Save` is still conditional (SR-18).

## Documentation

- [ ] Exported identifiers have doc comments stating the contract, not the
      signature.
- [ ] A structural decision here has an ADR, written **before** the code
      (NFR-14).
- [ ] An existing ADR was amended, never rewritten.
- [ ] `./.github/scripts/check-docs.sh` passes locally.
