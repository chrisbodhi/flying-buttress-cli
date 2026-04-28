package main

import (
	"context"
	"os"

	"buttress/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
