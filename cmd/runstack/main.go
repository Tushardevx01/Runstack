package main

import (
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		runCLI([]string{})
		return
	}

	command := os.Args[1]
	switch command {
	case "context":
		runContext(os.Args[2:])
	case "auth":
		runAuth(os.Args[2:])
	case "cp", "control-plane":
		runControlPlane(os.Args[2:])
	case "agent":
		runAgent(os.Args[2:])
	default:
		runCLI(os.Args[1:])
	}
}
