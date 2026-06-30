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
		"domain":   cmdDomain,
		"db":       cmdDB,
		"scale":    cmdScale,
		"health":   cmdHealth,
		"disk":     cmdDisk,
		"metrics":  cmdMetrics,
		"token":    cmdToken,
		"member":   cmdMember,
		"audit":    cmdAudit,
		"registry": cmdRegistry,
		"suspend":  cmdSuspend,
		"resume":   cmdResume,
		"info":     cmdInfo,
		"usage":    cmdUsage,
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
  uran login    --api URL --email EMAIL --password PASSWORD   (or --token PAT)
  uran token create --name NAME
  uran token list
  uran token rm  --id ID
  uran member list     --org ID
  uran member add      --org ID --email EMAIL [--role member]
  uran member set-role --org ID --user ID --role ROLE
  uran member rm       --org ID --user ID
  uran registry list --org ID
  uran registry add  --org ID --registry HOST --username U --password P
  uran registry rm   --org ID --id CRED_ID
  uran whoami
  uran deploy   --service ID [--commit SHA | --image REF]
  uran status   --deploy ID | --project ID
  uran info     --service ID
  uran logs     --deploy ID | --service ID
  uran metrics  --service ID
  uran audit
  uran rollback --deploy ID
  uran env list    --service ID
  uran env set     --service ID [--secret] KEY=VALUE
  uran env rm      --service ID KEY
  uran domain list --service ID
  uran domain add  --service ID DOMAIN
  uran domain rm   --service ID DOMAIN
  uran db create     --project ID NAME
  uran db list       --project ID
  uran db connection --database ID
  uran db rm         --database ID
  uran scale  --service ID [--replicas N] [--size small|medium|large] [--min N --max N]
  uran health  --service ID --path /healthz
  uran suspend --service ID
  uran resume  --service ID
  uran disk attach --service ID --size 1Gi --path /data
  uran disk detach --service ID
`)
}
