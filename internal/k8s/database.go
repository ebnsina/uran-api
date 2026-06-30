package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/ebnsina/uran-api/internal/instance"
)

// cnpgClusterGVR is the CloudNativePG Cluster resource.
var cnpgClusterGVR = schema.GroupVersionResource{
	Group:    "postgresql.cnpg.io",
	Version:  "v1",
	Resource: "clusters",
}

// dbProvisionTimeout bounds how long we wait for a Postgres cluster to become
// ready (image pull + initdb can take a couple of minutes).
const dbProvisionTimeout = 5 * time.Minute

// PostgresSpec is the desired managed-Postgres configuration.
type PostgresSpec struct {
	Instances int32  // number of nodes (>1 enables HA + a read endpoint)
	Size      string // instance.* size for CPU/memory
	Storage   string // PVC size, e.g. "1Gi"
	Backups   bool   // enable WAL archiving + scheduled backups to object storage
}

// ProvisionPostgres creates/updates a CloudNativePG cluster in the namespace
// (ensuring the namespace and its isolation policies exist first). With more
// than one instance, CNPG runs a primary plus standbys and exposes a
// load-balanced read-only service.
func (r *Reconciler) ProvisionPostgres(ctx context.Context, namespace, name string, spec PostgresSpec) error {
	if err := r.ensureNamespace(ctx, namespace); err != nil {
		return err
	}
	if err := r.applyNamespacePolicies(ctx, namespace); err != nil {
		return err
	}
	res := instance.Get(spec.Size)
	clusterSpec := map[string]any{
		"instances": int64(spec.Instances),
		"storage":   map[string]any{"size": spec.Storage},
		// Explicit resources so the pods satisfy the namespace ResourceQuota.
		"resources": map[string]any{
			"requests": map[string]any{"cpu": res.CPURequest, "memory": res.MemRequest},
			"limits":   map[string]any{"cpu": res.CPULimit, "memory": res.MemLimit},
		},
	}

	// Continuous backups (WAL archiving) to object storage, enabling PITR.
	if spec.Backups && r.BackupsEnabled() {
		if err := r.applyBackupCreds(ctx, namespace, name); err != nil {
			return err
		}
		clusterSpec["backup"] = r.barmanObjectStore(name)
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "Cluster",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       clusterSpec,
	}}
	if _, err := r.dyn.Resource(cnpgClusterGVR).Namespace(namespace).
		Apply(ctx, name, obj, applyOpts()); err != nil {
		return fmt.Errorf("apply postgres cluster %s: %w", name, err)
	}

	if spec.Backups && r.BackupsEnabled() {
		if err := r.applyScheduledBackup(ctx, namespace, name); err != nil {
			return err
		}
	}
	return nil
}

// PostgresReadURI returns the load-balanced read-only endpoint URI, derived from
// the read-write URI by swapping the CNPG service suffix.
func PostgresReadURI(rwURI, name string) string {
	return strings.Replace(rwURI, name+"-rw", name+"-ro", 1)
}

var cnpgPoolerGVR = schema.GroupVersionResource{
	Group:    "postgresql.cnpg.io",
	Version:  "v1",
	Resource: "poolers",
}

// poolerName is the PgBouncer pooler for a cluster.
func poolerName(cluster string) string { return cluster + "-pool" }

// PostgresPooledURI derives the pooled (PgBouncer) endpoint URI from the
// read-write URI.
func PostgresPooledURI(rwURI, cluster string) string {
	return strings.Replace(rwURI, cluster+"-rw", poolerName(cluster), 1)
}

// ReconcilePgBouncer creates or removes a PgBouncer pooler in front of a
// cluster's read-write endpoint.
func (r *Reconciler) ReconcilePgBouncer(ctx context.Context, namespace, cluster string, enabled bool) error {
	name := poolerName(cluster)
	if !enabled {
		err := r.dyn.Resource(cnpgPoolerGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if ignoreNotFound(err) != nil {
			return fmt.Errorf("delete pooler %s: %w", name, err)
		}
		return nil
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "Pooler",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec": map[string]any{
			"cluster":   map[string]any{"name": cluster},
			"instances": int64(1),
			"type":      "rw",
			"pgbouncer": map[string]any{"poolMode": "transaction"},
		},
	}}
	if _, err := r.dyn.Resource(cnpgPoolerGVR).Namespace(namespace).Apply(ctx, name, obj, applyOpts()); err != nil {
		return fmt.Errorf("apply pooler %s: %w", name, err)
	}
	return nil
}

// WaitPostgresReady polls until the cluster reports at least one ready instance.
func (r *Reconciler) WaitPostgresReady(ctx context.Context, namespace, name string) error {
	ctx, cancel := context.WithTimeout(ctx, dbProvisionTimeout)
	defer cancel()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		obj, err := r.dyn.Resource(cnpgClusterGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			ready, _, _ := unstructured.NestedInt64(obj.Object, "status", "readyInstances")
			if ready >= 1 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres cluster %s/%s not ready: %w", namespace, name, ctx.Err())
		case <-ticker.C:
		}
	}
}

// PostgresConnectionURI reads the application connection URI from the secret
// CloudNativePG generates for the cluster (<name>-app, key "uri").
func (r *Reconciler) PostgresConnectionURI(ctx context.Context, namespace, name string) (string, error) {
	secret, err := r.kube.CoreV1().Secrets(namespace).Get(ctx, name+"-app", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("read cluster secret %s-app: %w", name, err)
	}
	uri, ok := secret.Data["uri"]
	if !ok {
		return "", fmt.Errorf("cluster secret %s-app has no uri", name)
	}
	return string(uri), nil
}

// DeletePostgres removes a managed Postgres cluster and its pooler (if any).
func (r *Reconciler) DeletePostgres(ctx context.Context, namespace, name string) error {
	if err := r.dyn.Resource(cnpgPoolerGVR).Namespace(namespace).Delete(ctx, poolerName(name), metav1.DeleteOptions{}); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete pooler %s: %w", name, err)
	}
	err := r.dyn.Resource(cnpgClusterGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if ignoreNotFound(err) != nil {
		return fmt.Errorf("delete postgres cluster %s: %w", name, err)
	}
	return nil
}
