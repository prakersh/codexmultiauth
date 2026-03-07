package main

import (
	"os"

	"github.com/prakersh/codexmultiauth/cmd"
)

var execute = cmd.Execute
var exit = os.Exit

func main() {
	if err := execute(); err != nil {
		exit(1)
	}
}
