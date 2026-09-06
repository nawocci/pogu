package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nawocci/pogu/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pogu:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage())
	}
	dataDir := os.Getenv("POGU_DATA_DIR")
	if dataDir == "" {
		dataDir = config.DefaultDataDir
	}
	var err error
	args, dataDir, err = extractDataDir(args, dataDir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "init":
		return initCommand(dataDir, args[1:])
	case "serve":
		return serveCommand(dataDir, args[1:])
	case "status":
		return controlStatus(dataDir)
	case "provider":
		if len(args) > 1 && args[1] == "key" {
			return controlProviderKey(dataDir, args[2:])
		}
		return controlResource(dataDir, args[0], args[1:])
	case "model", "key":
		return controlResource(dataDir, args[0], args[1:])
	case "telemetry":
		return telemetryCommand(dataDir, args[1:])
	default:
		return fmt.Errorf("unknown command %q\n%s", args[0], usage())
	}
}

func usage() string {
	return `usage:
  pogu init --data-dir DIR --password PASSWORD
  pogu serve --data-dir DIR [--listen HOST:PORT]
  pogu status --data-dir DIR
  pogu provider list|create|get|update|delete|test ...
  pogu provider key list|create|get|update|enable|disable|delete|primary|test ...
  pogu model list|create|get|update|delete ...
  pogu key list|create|revoke ...
  pogu telemetry list [--limit N]`
}

func extractDataDir(args []string, defaultDir string) ([]string, string, error) {
	out := make([]string, 0, len(args))
	dataDir := defaultDir
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--data-dir":
			if i+1 >= len(args) || args[i+1] == "" {
				return nil, "", errors.New("--data-dir requires a value")
			}
			dataDir = args[i+1]
			i++
		case strings.HasPrefix(arg, "--data-dir="):
			dataDir = strings.TrimPrefix(arg, "--data-dir=")
			if dataDir == "" {
				return nil, "", errors.New("--data-dir requires a value")
			}
		default:
			out = append(out, arg)
		}
	}
	return out, dataDir, nil
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}
