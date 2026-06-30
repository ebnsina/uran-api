package k8s

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// ProvisionPostgres creates a single-instance CloudNativePG cluster in the
// namespace (ensuring the namespace and its isolation policies exist first).
func (r *Reconciler) ProvisionPostgres(ctx context.Context, namespace, name string) error {
	if err := r.ensureNamespace(ctx, namespace); err != nil {
		return err
	}
	if err := r.applyNamespacePolicies(ctx, namespace); err != nil {
		return err
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "Cluster",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"instances": int64(1),
			"storage":   map[string]any{"size": "1Gi"},
			// Explicit resources so the pods satisfy the namespace ResourceQuota.
			"resources": map[string]any{
				"requests": map[string]any{"cpu": "50m", "memory": "128Mi"},
				"limits":   map[string]any{"cpu": "500m", "memory": "512Mi"},
			},
		},
	}}
	if _, err := r.dyn.Resource(cnpgClusterGVR).Namespace(namespace).
		Apply(ctx, name, obj, applyOpts()); err != nil {
		return fmt.Errorf("apply postgres cluster %s: %w", name, err)
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

// DeletePostgres removes a managed Postgres cluster.
func (r *Reconciler) DeletePostgres(ctx context.Context, namespace, name string) error {
	err := r.dyn.Resource(cnpgClusterGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if ignoreNotFound(err) != nil {
		return fmt.Errorf("delete postgres cluster %s: %w", name, err)
	}
	return nil
}
