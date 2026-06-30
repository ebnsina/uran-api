// Command uran is the Uran CLI: log in, trigger deploys, stream build logs,
// manage env vars, and roll back — all against the control-plane API.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	commands := map[string]func([]string) error{
		"login":    cmdLogin,
		"whoami":   cmdWhoami,
		"deploy":   cmdDeploy,
		"status":   cmdStatus,
		"logs":     cmdLogs,
		"rollback": cmdRollback,
		"env":      cmdEnv,
	}

	cmd, ok := commands[os.Args[1]]
	if !ok {
		usage()
		os.Exit(2)
	}
	if err := cmd(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `uran — control plane CLI

Usage:
  uran login    --api URL --email EMAIL --password PASSWORD
  uran whoami
  uran deploy   --service ID [--commit SHA]
  uran status   --deploy ID
  uran logs     --deploy ID
  uran rollback --deploy ID
  uran env list --service ID
  uran env set  --service ID [--secret] KEY=VALUE
  uran env rm   --service ID KEY
`)
}
