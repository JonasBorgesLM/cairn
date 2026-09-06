module github.com/JonasBorgesLM/cairn/demo

// This module exists only to be docker-composed and curled (issue #51); it is
// never imported and never published, so it is free of the core's dependency
// policy (ADR-0001) -- moat's preset package and both cairn modules are
// exactly the point of it.
go 1.25.0

require (
	github.com/JonasBorgesLM/cairn v0.0.0-20260906061913-523af00bd46f
	github.com/JonasBorgesLM/cairn/redisstore v0.0.0-20260906061913-523af00bd46f
	github.com/JonasBorgesLM/moat v0.2.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
