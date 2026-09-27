package main

import "os"

func main() {
	os.Exit(runCLI(os.Args[1:]))
}
