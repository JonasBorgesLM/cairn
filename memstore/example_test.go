package memstore_test

import (
	"context"
	"fmt"

	"github.com/JonasBorgesLM/cairn"
	"github.com/JonasBorgesLM/cairn/memstore"
)

func Example() {
	store := memstore.New()
	dest, err := cairn.ParseDestination("https://example.com/")
	if err != nil {
		panic(err)
	}

	err = store.Save(context.Background(), &cairn.Link{Code: "abc1234567", Dest: dest})
	fmt.Println("Save:", err)

	link, err := store.Load(context.Background(), "abc1234567")
	if err != nil {
		panic(err)
	}
	fmt.Println("Load:", link.Code)

	// Output:
	// Save: <nil>
	// Load: abc1234567
}
