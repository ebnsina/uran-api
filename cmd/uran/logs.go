package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"strings"
)

// cmdLogs streams a deploy's build logs (Server-Sent Events) to stdout until
// the stream closes (build reaches a terminal state).
func cmdLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	deployID := fs.Int64("deploy", 0, "deploy id")
	_ = fs.Parse(args)
	if *deployID == 0 {
		return fmt.Errorf("usage: uran logs --deploy ID")
	}
	c, err := authed()
	if err != nil {
		return err
	}

	req, err := c.request(context.Background(), http.MethodGet, fmt.Sprintf("/v1/deploys/%d/logs", *deployID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("logs: %s", serverError(resp))
	}

	// Parse the minimal SSE we emit: "data: <line>" and "event: end".
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "data: "):
			fmt.Println(strings.TrimPrefix(line, "data: "))
		case strings.HasPrefix(line, "event: end"):
			// next data line carries the final status; keep reading
		}
	}
	return scanner.Err()
}
