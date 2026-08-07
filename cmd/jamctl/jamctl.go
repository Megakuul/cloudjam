package main

import (
	"os"

	"codeberg.org/megakuul/cloudjam/cmd/jamctl/app"
)

func main() {
	cmd := app.NewCmd()
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
