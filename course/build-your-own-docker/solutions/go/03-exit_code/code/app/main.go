package main

import (
	"os"
	"os/exec"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	cmd := exec.Command(command, args...)

	// Wire up stdout and stderr from child process
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	// Wire up exit codes from child process
	err := cmd.Run()
	if err != nil {
		os.Exit(err.(*exec.ExitError).ExitCode())
	}
}
