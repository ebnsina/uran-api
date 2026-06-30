package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// deployView mirrors the API's deploy JSON for printing.
type deployView struct {
	ID        int64  `json:"id"`
	ServiceID int64  `json:"service_id"`
	Status    string `json:"status"`
	CommitSHA string `json:"commit_sha"`
	Image     string `json:"image"`
}

type envVar struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// cmdLogin authenticates with email/password, or stores an API token directly
// (--token) for CI use.
func cmdLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	api := fs.String("api", "", "Uran API base URL, e.g. http://localhost:8099")
	email := fs.String("email", "", "account email")
	password := fs.String("password", "", "account password")
	token := fs.String("token", "", "personal access token (instead of email/password)")
	_ = fs.Parse(args)

	if *api == "" {
		return fmt.Errorf("usage: uran login --api URL [--email E --password P | --token TOKEN]")
	}
	if *token != "" {
		if err := saveCredentials(credentials{APIURL: *api, Token: *token}); err != nil {
			return err
		}
		fmt.Println("saved API token for", *api)
		return nil
	}
	if *email == "" || *password == "" {
		return fmt.Errorf("usage: uran login --api URL --email EMAIL --password PASSWORD")
	}

	c := newClient(credentials{APIURL: *api})
	var resp struct {
		Token string `json:"token"`
	}
	body := map[string]string{"email": *email, "password": *password}
	if err := c.do(context.Background(), http.MethodPost, "/v1/auth/login", body, &resp); err != nil {
		return err
	}
	if err := saveCredentials(credentials{APIURL: *api, Token: resp.Token}); err != nil {
		return err
	}
	fmt.Println("logged in to", *api)
	return nil
}

type apiToken struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
	Token  string `json:"token"`
}

// cmdToken dispatches token subcommands: create, list, rm.
func cmdToken(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: uran token <create|list|rm> ...")
	}
	switch args[0] {
	case "create":
		return cmdTokenCreate(args[1:])
	case "list":
		return cmdTokenList(args[1:])
	case "rm":
		return cmdTokenRm(args[1:])
	default:
		return fmt.Errorf("unknown token subcommand %q", args[0])
	}
}

func cmdTokenCreate(args []string) error {
	fs := flag.NewFlagSet("token create", flag.ExitOnError)
	name := fs.String("name", "", "token name")
	_ = fs.Parse(args)
	if *name == "" {
		return fmt.Errorf("usage: uran token create --name NAME")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var t apiToken
	if err := c.do(context.Background(), http.MethodPost, "/v1/tokens", map[string]string{"name": *name}, &t); err != nil {
		return err
	}
	fmt.Println(t.Token)
	fmt.Fprintln(os.Stderr, "(store this now — it won't be shown again)")
	return nil
}

func cmdTokenList(args []string) error {
	c, err := authed()
	if err != nil {
		return err
	}
	var tokens []apiToken
	if err := c.do(context.Background(), http.MethodGet, "/v1/tokens", nil, &tokens); err != nil {
		return err
	}
	if len(tokens) == 0 {
		fmt.Println("(no tokens)")
		return nil
	}
	for _, t := range tokens {
		fmt.Printf("%d  %-20s %s...\n", t.ID, t.Name, t.Prefix)
	}
	return nil
}

func cmdTokenRm(args []string) error {
	fs := flag.NewFlagSet("token rm", flag.ExitOnError)
	id := fs.Int64("id", 0, "token id")
	_ = fs.Parse(args)
	if *id == 0 {
		return fmt.Errorf("usage: uran token rm --id ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodDelete, fmt.Sprintf("/v1/tokens/%d", *id), nil, nil); err != nil {
		return err
	}
	fmt.Printf("deleted token %d\n", *id)
	return nil
}

// cmdWhoami prints the current user.
func cmdWhoami(args []string) error {
	c, err := authed()
	if err != nil {
		return err
	}
	var user map[string]any
	if err := c.do(context.Background(), http.MethodGet, "/v1/me", nil, &user); err != nil {
		return err
	}
	fmt.Printf("%v <%v>\n", user["name"], user["email"])
	return nil
}

// cmdDeploy triggers a deploy for a service: a Git build by default, or a
// prebuilt image when --image is given.
func cmdDeploy(args []string) error {
	fs := flag.NewFlagSet("deploy", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	commit := fs.String("commit", "", "optional commit sha (Git build)")
	image := fs.String("image", "", "deploy a prebuilt image instead of building")
	_ = fs.Parse(args)
	if *service == 0 {
		return fmt.Errorf("usage: uran deploy --service ID [--commit SHA | --image REF]")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var d deployView
	if *image != "" {
		body := map[string]string{"image": *image}
		if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/services/%d/image-deploys", *service), body, &d); err != nil {
			return err
		}
		fmt.Printf("deploying image %s as deploy %d\n", *image, d.ID)
		return nil
	}
	body := map[string]string{"commit_sha": *commit}
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/services/%d/deploys", *service), body, &d); err != nil {
		return err
	}
	fmt.Printf("queued deploy %d for service %d\n", d.ID, d.ServiceID)
	fmt.Printf("stream logs: uran logs --deploy %d\n", d.ID)
	return nil
}

// cmdStatus prints a single deploy.
func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	deployID := fs.Int64("deploy", 0, "deploy id")
	_ = fs.Parse(args)
	if *deployID == 0 {
		return fmt.Errorf("usage: uran status --deploy ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var d deployView
	if err := c.do(context.Background(), http.MethodGet, fmt.Sprintf("/v1/deploys/%d", *deployID), nil, &d); err != nil {
		return err
	}
	fmt.Printf("deploy %d  status=%s  image=%s\n", d.ID, d.Status, d.Image)
	return nil
}

// cmdRollback re-deploys a prior deploy's image.
func cmdRollback(args []string) error {
	fs := flag.NewFlagSet("rollback", flag.ExitOnError)
	deployID := fs.Int64("deploy", 0, "deploy id to roll back to")
	_ = fs.Parse(args)
	if *deployID == 0 {
		return fmt.Errorf("usage: uran rollback --deploy ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var d deployView
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/deploys/%d/rollback", *deployID), nil, &d); err != nil {
		return err
	}
	fmt.Printf("rollback created deploy %d (image %s)\n", d.ID, d.Image)
	return nil
}

type domain struct {
	ID     int64  `json:"id"`
	Domain string `json:"domain"`
}

type database struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Engine string `json:"engine"`
	Status string `json:"status"`
}

// cmdDB dispatches database subcommands: create, list, connection, rm.
func cmdDB(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: uran db <create|list|connection|rm> ...")
	}
	switch args[0] {
	case "create":
		return cmdDBCreate(args[1:])
	case "list":
		return cmdDBList(args[1:])
	case "connection":
		return cmdDBConnection(args[1:])
	case "rm":
		return cmdDBRm(args[1:])
	default:
		return fmt.Errorf("unknown db subcommand %q", args[0])
	}
}

func cmdDBCreate(args []string) error {
	fs := flag.NewFlagSet("db create", flag.ExitOnError)
	project := fs.Int64("project", 0, "project id")
	_ = fs.Parse(args)
	rest := fs.Args()
	if *project == 0 || len(rest) != 1 {
		return fmt.Errorf("usage: uran db create --project ID NAME")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var db database
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/projects/%d/databases", *project), map[string]string{"name": rest[0]}, &db); err != nil {
		return err
	}
	fmt.Printf("creating database %d (%s) — check: uran db connection --database %d\n", db.ID, db.Status, db.ID)
	return nil
}

func cmdDBList(args []string) error {
	fs := flag.NewFlagSet("db list", flag.ExitOnError)
	project := fs.Int64("project", 0, "project id")
	_ = fs.Parse(args)
	if *project == 0 {
		return fmt.Errorf("usage: uran db list --project ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var dbs []database
	if err := c.do(context.Background(), http.MethodGet, fmt.Sprintf("/v1/projects/%d/databases", *project), nil, &dbs); err != nil {
		return err
	}
	if len(dbs) == 0 {
		fmt.Println("(no databases)")
		return nil
	}
	for _, d := range dbs {
		fmt.Printf("%d  %-20s %-10s %s\n", d.ID, d.Name, d.Engine, d.Status)
	}
	return nil
}

func cmdDBConnection(args []string) error {
	fs := flag.NewFlagSet("db connection", flag.ExitOnError)
	id := fs.Int64("database", 0, "database id")
	_ = fs.Parse(args)
	if *id == 0 {
		return fmt.Errorf("usage: uran db connection --database ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var resp struct {
		URI string `json:"uri"`
	}
	if err := c.do(context.Background(), http.MethodGet, fmt.Sprintf("/v1/databases/%d/connection", *id), nil, &resp); err != nil {
		return err
	}
	fmt.Println(resp.URI)
	return nil
}

func cmdDBRm(args []string) error {
	fs := flag.NewFlagSet("db rm", flag.ExitOnError)
	id := fs.Int64("database", 0, "database id")
	_ = fs.Parse(args)
	if *id == 0 {
		return fmt.Errorf("usage: uran db rm --database ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodDelete, fmt.Sprintf("/v1/databases/%d", *id), nil, nil); err != nil {
		return err
	}
	fmt.Printf("deleted database %d\n", *id)
	return nil
}

// cmdDomain dispatches domain subcommands: list, add, rm.
func cmdDomain(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: uran domain <list|add|rm> --service ID ...")
	}
	switch args[0] {
	case "list":
		return cmdDomainList(args[1:])
	case "add":
		return cmdDomainAdd(args[1:])
	case "rm":
		return cmdDomainRm(args[1:])
	default:
		return fmt.Errorf("unknown domain subcommand %q", args[0])
	}
}

func cmdDomainList(args []string) error {
	fs := flag.NewFlagSet("domain list", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	_ = fs.Parse(args)
	if *service == 0 {
		return fmt.Errorf("usage: uran domain list --service ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var domains []domain
	if err := c.do(context.Background(), http.MethodGet, fmt.Sprintf("/v1/services/%d/domains", *service), nil, &domains); err != nil {
		return err
	}
	if len(domains) == 0 {
		fmt.Println("(no custom domains)")
		return nil
	}
	for _, d := range domains {
		fmt.Println(d.Domain)
	}
	return nil
}

func cmdDomainAdd(args []string) error {
	fs := flag.NewFlagSet("domain add", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	_ = fs.Parse(args)
	rest := fs.Args()
	if *service == 0 || len(rest) != 1 {
		return fmt.Errorf("usage: uran domain add --service ID DOMAIN")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/services/%d/domains", *service), map[string]string{"domain": rest[0]}, nil); err != nil {
		return err
	}
	fmt.Printf("added %s (apply with a deploy or rollback)\n", rest[0])
	return nil
}

func cmdDomainRm(args []string) error {
	fs := flag.NewFlagSet("domain rm", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	_ = fs.Parse(args)
	rest := fs.Args()
	if *service == 0 || len(rest) != 1 {
		return fmt.Errorf("usage: uran domain rm --service ID DOMAIN")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodDelete, fmt.Sprintf("/v1/services/%d/domains/%s", *service, rest[0]), nil, nil); err != nil {
		return err
	}
	fmt.Printf("removed %s\n", rest[0])
	return nil
}

type podMetric struct {
	Pod           string `json:"pod"`
	CPUMillicores int64  `json:"cpu_millicores"`
	MemoryBytes   int64  `json:"memory_bytes"`
}

// cmdMetrics prints current CPU/memory usage per pod for a service.
func cmdMetrics(args []string) error {
	fs := flag.NewFlagSet("metrics", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	_ = fs.Parse(args)
	if *service == 0 {
		return fmt.Errorf("usage: uran metrics --service ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var metrics []podMetric
	if err := c.do(context.Background(), http.MethodGet, fmt.Sprintf("/v1/services/%d/metrics", *service), nil, &metrics); err != nil {
		return err
	}
	if len(metrics) == 0 {
		fmt.Println("(no metrics yet)")
		return nil
	}
	for _, m := range metrics {
		fmt.Printf("%-40s %5dm CPU  %6dMi\n", m.Pod, m.CPUMillicores, m.MemoryBytes/(1024*1024))
	}
	return nil
}

// cmdScale updates a service's replicas, instance size, and autoscaling bounds.
func cmdScale(args []string) error {
	fs := flag.NewFlagSet("scale", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	replicas := fs.Int("replicas", 1, "fixed replica count (ignored when autoscaling)")
	size := fs.String("size", "small", "instance size: small|medium|large")
	min := fs.Int("min", 0, "autoscale min replicas (0 disables autoscaling)")
	max := fs.Int("max", 0, "autoscale max replicas (0 disables autoscaling)")
	_ = fs.Parse(args)
	if *service == 0 {
		return fmt.Errorf("usage: uran scale --service ID [--replicas N] [--size small|medium|large] [--min N --max N]")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	body := map[string]any{"replicas": *replicas, "instance_size": *size, "min_replicas": *min, "max_replicas": *max}
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/services/%d/scale", *service), body, nil); err != nil {
		return err
	}
	if *max > 0 {
		fmt.Printf("autoscaling %d–%d (%s); applying\n", *min, *max, *size)
	} else {
		fmt.Printf("scaled to %d × %s; applying\n", *replicas, *size)
	}
	return nil
}

// cmdHealth sets a service's HTTP health-check path (empty for a TCP check).
func cmdHealth(args []string) error {
	fs := flag.NewFlagSet("health", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	path := fs.String("path", "", "HTTP health-check path, e.g. /healthz")
	_ = fs.Parse(args)
	if *service == 0 {
		return fmt.Errorf("usage: uran health --service ID --path /healthz")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/services/%d/health", *service), map[string]string{"path": *path}, nil); err != nil {
		return err
	}
	fmt.Printf("health check set to %q; applying\n", *path)
	return nil
}

// cmdDisk dispatches disk subcommands: attach, detach.
func cmdDisk(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: uran disk <attach|detach> --service ID ...")
	}
	switch args[0] {
	case "attach":
		return cmdDiskAttach(args[1:])
	case "detach":
		return cmdDiskDetach(args[1:])
	default:
		return fmt.Errorf("unknown disk subcommand %q", args[0])
	}
}

func cmdDiskAttach(args []string) error {
	fs := flag.NewFlagSet("disk attach", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	size := fs.String("size", "", "disk size, e.g. 1Gi")
	path := fs.String("path", "", "mount path, e.g. /data")
	_ = fs.Parse(args)
	if *service == 0 || *size == "" || *path == "" {
		return fmt.Errorf("usage: uran disk attach --service ID --size 1Gi --path /data")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/services/%d/disk", *service), map[string]string{"size": *size, "path": *path}, nil); err != nil {
		return err
	}
	fmt.Printf("attached %s disk at %s (single replica); applying\n", *size, *path)
	return nil
}

func cmdDiskDetach(args []string) error {
	fs := flag.NewFlagSet("disk detach", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	_ = fs.Parse(args)
	if *service == 0 {
		return fmt.Errorf("usage: uran disk detach --service ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodDelete, fmt.Sprintf("/v1/services/%d/disk", *service), nil, nil); err != nil {
		return err
	}
	fmt.Println("disk detached; applying")
	return nil
}

// cmdEnv dispatches env subcommands: list, set, rm.
func cmdEnv(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: uran env <list|set|rm> --service ID ...")
	}
	switch args[0] {
	case "list":
		return cmdEnvList(args[1:])
	case "set":
		return cmdEnvSet(args[1:])
	case "rm":
		return cmdEnvRm(args[1:])
	default:
		return fmt.Errorf("unknown env subcommand %q", args[0])
	}
}

func cmdEnvList(args []string) error {
	fs := flag.NewFlagSet("env list", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	_ = fs.Parse(args)
	if *service == 0 {
		return fmt.Errorf("usage: uran env list --service ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	var vars []envVar
	if err := c.do(context.Background(), http.MethodGet, fmt.Sprintf("/v1/services/%d/env", *service), nil, &vars); err != nil {
		return err
	}
	if len(vars) == 0 {
		fmt.Println("(no env vars)")
		return nil
	}
	for _, v := range vars {
		val := v.Value
		if v.Secret {
			val = "********"
		}
		fmt.Printf("%s=%s\n", v.Key, val)
	}
	return nil
}

func cmdEnvSet(args []string) error {
	fs := flag.NewFlagSet("env set", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	secret := fs.Bool("secret", false, "mark the value as secret")
	_ = fs.Parse(args)
	rest := fs.Args()
	if *service == 0 || len(rest) != 1 || !strings.Contains(rest[0], "=") {
		return fmt.Errorf("usage: uran env set --service ID [--secret] KEY=VALUE")
	}
	key, value, _ := strings.Cut(rest[0], "=")
	c, err := authed()
	if err != nil {
		return err
	}
	body := map[string]any{"key": key, "value": value, "secret": *secret}
	if err := c.do(context.Background(), http.MethodPost, fmt.Sprintf("/v1/services/%d/env", *service), body, nil); err != nil {
		return err
	}
	fmt.Printf("set %s (apply with: uran rollback --deploy <last-live-deploy>)\n", key)
	return nil
}

func cmdEnvRm(args []string) error {
	fs := flag.NewFlagSet("env rm", flag.ExitOnError)
	service := fs.Int64("service", 0, "service id")
	_ = fs.Parse(args)
	rest := fs.Args()
	if *service == 0 || len(rest) != 1 {
		return fmt.Errorf("usage: uran env rm --service ID KEY")
	}
	c, err := authed()
	if err != nil {
		return err
	}
	if err := c.do(context.Background(), http.MethodDelete, fmt.Sprintf("/v1/services/%d/env/%s", *service, rest[0]), nil, nil); err != nil {
		return err
	}
	fmt.Printf("removed %s\n", rest[0])
	return nil
}

// authed loads the saved session into a client.
func authed() (*client, error) {
	creds, err := loadCredentials()
	if err != nil {
		return nil, err
	}
	return newClient(creds), nil
}
