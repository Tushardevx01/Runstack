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
	}

	command := os.Args[1]
	switch command {
	case "cp", "control-plane":
		runControlPlane()
	case "agent":
		runAgent()
	default:
		runCLI(os.Args[1:])
	}
}
