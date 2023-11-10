//go:build linux

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	command := os.Args[3]
	args := os.Args[4:]

	tempDir, err := createIsolation(command)
	defer os.RemoveAll(tempDir)

	checkErr(err)

	cmd := exec.Command(command, args...)

	// Wire up stdout and stderr from child process
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	// This is required because, if any one of them is nil, cmd.Run() will throw an error saying it
	// needs /dev/null in the new root
	cmd.Stdin = os.Stdin

	// Isolate the pid namespace, so that the processes running inside the containerised temp folder
	// cannot access the local/parent machine's process and make any destructive changes
	// CloneFlags is not available on Mac -- need to set: a couple of directives at the top of the file
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWPID,
	}

	// Wire up exit codes from child process
	err = cmd.Run()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(err.(*exec.ExitError).ExitCode())
	}
}

// We want to ensure isolation from the local filesystem so that the command does not perform anything untoward or risky.
// So, we can create a temporary directory, make it root, copy the executable and run the same
func createIsolation(executable string) (string, error) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "docker-codecrafters")

	if err != nil {
		return "", err
	}

	// Create the required directories in the temp dir
	os.MkdirAll(tempDir+filepath.Dir(executable), os.ModeAppend)

	// Let's copy the executable first
	copy(executable, tempDir+executable)
	// Set the correct permissions
	if err = os.Chmod(tempDir+executable, 0755); err != nil {
		return tempDir, err
	}

	// Move to the temp dir
	syscall.Chdir(tempDir)
	if err != nil {
		return tempDir, err
	}
	// Make temp dir as the root
	err = syscall.Chroot(tempDir)
	if err != nil {
		return tempDir, err
	}

	return tempDir, nil
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func copy(src string, dst string) {
	// Read all content of src to data, may cause OOM for a large file.
	data, err := os.ReadFile(src)
	checkErr(err)
	// Write data to dst
	err = os.WriteFile(dst, data, 0644)
	checkErr(err)
}
