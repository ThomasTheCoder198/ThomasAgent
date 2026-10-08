package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/thomasthecoder198/thomastheragx/contracts/internal/apptoken"
)

const filePerm = 0o644

func main() {
	out := flag.String("out", "app-token.json", "output file")
	flag.Parse()
	rendered, err := apptoken.Render()
	if err == nil {
		err = os.WriteFile(*out, rendered, filePerm)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "apptokengen:", err)
		os.Exit(1)
	}
}
