package main

import (

	// Uncomment this block to pass the first stage!
	"os"
	"os/exec"
)

// Usage: your_docker.sh run <image> <command> <arg1> <arg2> ...
func main() {
	// You can use print statements as follows for debugging, they'll be visible when running tests.
	// fmt.Println("Logs from your program will appear here!")

	// Uncomment this block to pass the first stage!
	//
	command := os.Args[3]
	args := os.Args[4:len(os.Args)]

	cmd := exec.Command(command, args...)

	// Need to pipe the stderr and stdout data. We cannout use Output -> does not provide stderr,
	// or CombinedOutput -> combines stdout and stderr, hence making it difficult to differentiate.

	// stdout, err := cmd.StdoutPipe()
	// if err != nil {
	// 	fmt.Printf("Err: %v", err)
	// 	os.Exit(1)
	// }
	// stderr, err := cmd.StderrPipe()
	// if err != nil {
	// 	fmt.Printf("Err: %v", err)
	// 	os.Exit(1)
	// }

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	// if err = cmd.Run(); err != nil {
	// 	log.Fatal(err)
	// }

	_ = cmd.Run()

	// Read the stderr and stdout pipes completely
	// out := io.Read

	// fmt.Print(string(output))
}
