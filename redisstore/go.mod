module github.com/JonasBorgesLM/cairn/redisstore

// This module carries go-redis and, for the integration suite, testcontainers
// (NFR-10). That is what a satellite module is for: a consumer who brings their
// own cairn.Store never sees these in their dependency graph (ADR-0001).
//
// The floor below is currently the same as the core's only because this module
// has no dependencies yet. When go-redis and testcontainers arrive, whatever
// floor they impose goes here — and it is recorded as an IMPOSITION, not an
// endorsement. The core's floor never follows it: nothing imports a satellite
// to be stranded by it, and the two questions are independent (NFR-02).
go 1.24
