package main

import "os"

// Deferred cleanup finishes before main translates the result into a process exit code.
func main() { os.Exit(runProcess()) }
