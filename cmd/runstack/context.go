package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/Tushardevx01/runstack/internal/config"
)

func runContext(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: runstack context [list|use|add]")
		os.Exit(1)
	}

	cmd := args[0]
	switch cmd {
	case "list":
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if len(cfg.Contexts) == 0 {
			fmt.Println("No contexts configured.")
			return
		}
		fmt.Println("CONTEXTS:")
		for _, ctx := range cfg.Contexts {
			prefix := "  "
			if ctx.Name == cfg.CurrentContext {
				prefix = "* "
			}
			fmt.Printf("%s%s\t%s\n", prefix, ctx.Name, ctx.Endpoint)
		}
	case "add":
		if len(args) < 4 || args[2] != "--endpoint" {
			fmt.Println("Usage: runstack context add <name> --endpoint <url> [--token <token>]")
			os.Exit(1)
		}
		name := args[1]
		endpoint := args[3]
		token := ""
		if len(args) >= 6 && args[4] == "--token" {
			token = args[5]
		}

		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		// Update or append
		found := false
		for i, ctx := range cfg.Contexts {
			if ctx.Name == name {
				cfg.Contexts[i].Endpoint = endpoint
				cfg.Contexts[i].Token = token
				found = true
				break
			}
		}
		if !found {
			cfg.Contexts = append(cfg.Contexts, config.Context{
				Name:     name,
				Endpoint: endpoint,
				Token:    token,
			})
		}
		if cfg.CurrentContext == "" {
			cfg.CurrentContext = name
		}

		if err := config.SaveConfig(cfg); err != nil {
			fmt.Printf("Failed to save config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Context '%s' added/updated.\n", name)

	case "use":
		if len(args) < 2 {
			fmt.Println("Usage: runstack context use <name>")
			os.Exit(1)
		}
		name := args[1]
		cfg, err := config.LoadConfig()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, ctx := range cfg.Contexts {
			if ctx.Name == name {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Context '%s' not found.\n", name)
			os.Exit(1)
		}
		cfg.CurrentContext = name
		if err := config.SaveConfig(cfg); err != nil {
			fmt.Printf("Failed to save config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Switched to context '%s'.\n", name)
	}
}

func runAuth(args []string) {
	if len(args) < 2 || args[0] != "token" || args[1] != "create" {
		fmt.Println("Usage: runstack auth token create")
		os.Exit(1)
	}
	b := make([]byte, 32)
	rand.Read(b)
	fmt.Println(hex.EncodeToString(b))
}
