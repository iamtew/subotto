// Package main is the entry point for Subotto.
//
// Meat Bag: this is where the program starts when you run `just run` or
// the built binary. Right now Phase 0 only prints a hello message so we
// know the scaffold works. Later phases will wire up config, Discord,
// YouTube, SQLite, and the Admin UI from here.
package main

import "fmt"

func main() {
	// Phase 0: prove the binary runs. Real startup logic comes in Phase 1+.
	fmt.Println("Subotto is online. Clanker stands ready, Meat Bag.")
}
