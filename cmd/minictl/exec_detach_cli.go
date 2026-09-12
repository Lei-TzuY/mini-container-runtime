package main

import (
	"fmt"
	"os"

	"minicontainer/internal/container"
	"minicontainer/internal/state"
)

// init handles the detached exec form before main dispatches the ordinary exec
// path. ExecDetached re-execs minictl without -d, so the child continues through
// cmdExec and the managed foreground lifecycle rather than recursing here.
func init() {
	if len(os.Args) < 2 || os.Args[1] != "exec" {
		return
	}
	detached, id, command, err := parseDetachedExecArgs(os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if !detached {
		return
	}

	store, err := openStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: state store: %v\n", err)
		os.Exit(1)
	}
	rec, err := store.Resolve(id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	rec, err = container.ReconcileContainerState(store, rec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: reconcile container: %v\n", err)
		os.Exit(1)
	}
	if rec.Status != state.StatusRunning {
		fmt.Fprintf(os.Stderr, "error: container %s is %s (must be running)\n", shortContainerID(rec.ID), rec.Status)
		os.Exit(1)
	}

	pid, err := container.ExecDetached(store, rec.ID, command)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exec error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%d\n", pid)
	os.Exit(0)
}

func parseDetachedExecArgs(args []string) (bool, string, []string, error) {
	if len(args) == 0 || (args[0] != "-d" && args[0] != "--detach") {
		return false, "", nil, nil
	}
	if len(args) < 3 || args[1] == "" || args[2] == "" {
		return true, "", nil, fmt.Errorf("Usage: minictl exec -d <id> <command> [args...]")
	}
	return true, args[1], append([]string(nil), args[2:]...), nil
}

func shortContainerID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
