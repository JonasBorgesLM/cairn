package redisstore_test

import (
	"fmt"

	"github.com/JonasBorgesLM/cairn/redisstore"
)

// Example shows construction only: New does not itself connect (go-redis
// dials lazily, on the first command), so this runs safely without a Redis
// server — the same reason NFR-10's real round trips live in the
// integration suite behind the "integration" build tag instead of here.
func Example() {
	store, err := redisstore.New("localhost:6379")
	if err != nil {
		panic(err)
	}
	defer store.Close()

	fmt.Println("store created")

	// Output: store created
}
