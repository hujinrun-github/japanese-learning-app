package main

import (
	"os"

	"japanese-learning-app/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
