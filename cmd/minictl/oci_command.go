package main

import (
	"fmt"
	"os"
)

func init() {
	if os.Getenv("MINICONTAINER_INIT") == "1" || os.Getenv("MINICONTAINER_EXEC") == "1" {
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "oci-run" {
		cmdOCIRunSafe(os.Args[2:])
		os.Exit(0)
	}
}

type ociRunCommandDeps struct {
	run func(string) (string, error)
}

func parseOCIRunCommandArgs(args []string) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("expected exactly one OCI bundle directory")
	}
	if args[0] == "" {
		return "", fmt.Errorf("OCI bundle directory is empty")
	}
	return args[0], nil
}

func runOCIBundleCommand(args []string, deps ociRunCommandDeps) (string, error) {
	bundle, err := parseOCIRunCommandArgs(args)
	if err != nil {
		return "", err
	}
	if deps.run == nil {
		return "", fmt.Errorf("OCI run command dependencies are incomplete")
	}
	return deps.run(bundle)
}

func cmdOCIRunSafe(args []string) {
	id, err := runOCIBundleCommand(args, ociRunCommandDeps{run: runOCIBundle})
	if err != nil {
		fmt.Fprintf(os.Stderr, "oci-run error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: minictl oci-run <bundle-dir>")
		os.Exit(runCommandExitCode(err))
	}
	fmt.Printf("%s\n", shortContainerID(id))
}
