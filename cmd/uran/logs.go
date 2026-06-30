package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// cmdLogs streams build logs for a deploy (--deploy) or live runtime logs for a
// service (--service).
func cmdLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	deployID := fs.Int64("deploy", 0, "deploy id (build logs)")
	serviceID := fs.Int64("service", 0, "service id (runtime logs)")
	_ = fs.Parse(args)
	if *deployID == 0 && *serviceID == 0 {
		return fmt.Errorf("usage: uran logs --deploy ID | --service ID")
	}
	if *serviceID != 0 {
		return streamRuntimeLogs(*serviceID)
	}
	return streamBuildLogs(*deployID)
}

// streamBuildLogs consumes the deploy build-log SSE stream.
func streamBuildLogs(deployID int64) error {
	c, err := authed()
	if err != nil {
		return err
	}
	req, err := c.request(context.Background(), http.MethodGet, fmt.Sprintf("/v1/deploys/%d/logs", deployID), nil)
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
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if after, ok := strings.CutPrefix(line, "data: "); ok {
			fmt.Println(after)
		}
	}
	return scanner.Err()
}

// streamRuntimeLogs follows a service's running pod logs (plain text).
func streamRuntimeLogs(serviceID int64) error {
	c, err := authed()
	if err != nil {
		return err
	}
	req, err := c.request(context.Background(), http.MethodGet, fmt.Sprintf("/v1/services/%d/runtime-logs", serviceID), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("runtime logs: %s", serverError(resp))
	}
	_, err = io.Copy(os.Stdout, resp.Body)
	return err
}
