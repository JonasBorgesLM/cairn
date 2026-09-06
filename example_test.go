package cairn_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/memstore"
)

// Example shows the whole lifecycle: create, resolve, revoke, resolve again.
// It needs no Redis — memstore is enough to exercise the full flow.
func Example() {
	s, err := cairn.New(memstore.New(), cairn.WithPolicy(allowPolicy{}))
	if err != nil {
		panic(err)
	}

	link, err := s.Create(context.Background(), "https://example.com/docs")
	if err != nil {
		panic(err)
	}

	resolved, err := s.Resolve(context.Background(), link.Code)
	if err != nil {
		panic(err)
	}
	fmt.Println("resolved:", resolved.Dest.Host())

	if err = s.Revoke(context.Background(), link.Code); err != nil {
		panic(err)
	}

	_, err = s.Resolve(context.Background(), link.Code)
	fmt.Println("after revoke:", errors.Is(err, cairn.ErrLinkRevoked))

	// Output:
	// resolved: example.com
	// after revoke: true
}

func ExampleShortener_Create_vanity() {
	s, err := cairn.New(memstore.New(),
		cairn.WithPolicy(allowPolicy{}),
		cairn.WithVanity(3, 8, []string{"admin"}), // excludes the default generated length, 10 (SR-04)
	)
	if err != nil {
		panic(err)
	}

	link, err := s.Create(context.Background(), "https://example.com/launch",
		cairn.WithVanityCode(cairn.Code("launch")),
	)
	if err != nil {
		panic(err)
	}

	fmt.Println(link.Code, link.Vanity)

	// Output: launch true
}
