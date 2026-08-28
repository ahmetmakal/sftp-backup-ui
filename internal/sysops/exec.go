package sysops

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
)

// run executes name with args, never through a shell, and returns combined
// stdout+stderr. Every invocation is logged so the admin can audit exactly
// which system-mutating commands this app ran.
func run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	log.Printf("sysops: running %s %v", name, args)
	err := cmd.Run()
	output := out.String()
	if err != nil {
		log.Printf("sysops: %s %v failed: %v (output: %s)", name, args, err, output)
		return output, fmt.Errorf("%s %v: %w: %s", name, args, err, output)
	}
	return output, nil
}
