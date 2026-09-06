module github.com/JonasBorgesLM/cairn/redisstore

// This module carries go-redis and, for the integration suite, testcontainers
// (NFR-10). That is what a satellite module is for: a consumer who brings their
// own cairn.Store never sees these in their dependency graph (ADR-0001).
//
// The floor below moved to 1.25.0 the moment testcontainers-go (v0.44.0, for
// the integration suite of #38) arrived: that is its own floor, and go-redis
// asks for nothing higher than the core's 1.24. Recorded as an IMPOSITION,
// not an endorsement — the core's floor never follows it, because nothing
// imports a satellite to be stranded by it, and the two questions are
// independent (NFR-02).
go 1.25.0

require github.com/JonasBorgesLM/cairn v0.0.0-20260906043424-dd96c8aa595a

require github.com/JonasBorgesLM/moat v0.2.0 // indirect
