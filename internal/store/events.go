package store

import (
	"context"
	"fmt"
)

// DeployChannel is the Postgres NOTIFY channel on which queued deploy IDs are
// published. The builder LISTENs on it to pick up work.
const DeployChannel = "uran_deploys"

// DeploymentChannel is the Postgres NOTIFY channel on which deploy IDs whose
// image is built and ready (status "deploying") are published. The controller
// LISTENs on it to reconcile them onto the cluster.
const DeploymentChannel = "uran_deployments"

// TeardownChannel is the Postgres NOTIFY channel on which preview teardown
// requests are published (payload "<serviceID>:<prNumber>"). The controller
// LISTENs on it to delete the preview's cluster objects.
const TeardownChannel = "uran_teardowns"

// DatabaseChannel carries IDs of databases to provision (payload "<dbID>").
const DatabaseChannel = "uran_databases"

// DatabaseTeardownChannel carries databases to deprovision, as the payload
// "<namespace>:<clusterName>" (the DB row is deleted by the API first).
const DatabaseTeardownChannel = "uran_db_teardowns"

// Notify publishes a payload on a NOTIFY channel using pg_notify, which (unlike
// the NOTIFY statement) accepts the channel name as a bind parameter.
func (s *Store) Notify(ctx context.Context, channel, payload string) error {
	_, err := s.pool.Exec(ctx, `SELECT pg_notify($1, $2)`, channel, payload)
	return err
}

// Listen acquires a dedicated connection, issues LISTEN on the channel, and
// invokes handler for every notification until ctx is cancelled. The channel
// name is interpolated as an identifier, so callers must pass a trusted
// constant (e.g. DeployChannel), never user input.
func (s *Store) Listen(ctx context.Context, channel string, handler func(payload string)) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire listen conn: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+pgQuoteIdent(channel)); err != nil {
		return fmt.Errorf("listen %s: %w", channel, err)
	}

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err // ctx cancelled or connection lost
		}
		handler(n.Payload)
	}
}

// pgQuoteIdent quotes a Postgres identifier by doubling embedded quotes.
func pgQuoteIdent(ident string) string {
	out := make([]byte, 0, len(ident)+2)
	out = append(out, '"')
	for i := 0; i < len(ident); i++ {
		if ident[i] == '"' {
			out = append(out, '"')
		}
		out = append(out, ident[i])
	}
	out = append(out, '"')
	return string(out)
}
