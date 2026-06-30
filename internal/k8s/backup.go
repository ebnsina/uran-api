package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
)

var (
	cnpgBackupGVR          = schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "backups"}
	cnpgScheduledBackupGVR = schema.GroupVersionResource{Group: "postgresql.cnpg.io", Version: "v1", Resource: "scheduledbackups"}
)

func backupCredsName(cluster string) string { return cluster + "-backup-creds" }

// BackupsEnabled reports whether an object store is configured.
func (r *Reconciler) BackupsEnabled() bool { return r.backup.Bucket != "" }

// barmanObjectStore returns the CNPG backup stanza for a cluster, pointing at
// the configured object store.
func (r *Reconciler) barmanObjectStore(cluster string) map[string]any {
	return map[string]any{
		"barmanObjectStore": map[string]any{
			"destinationPath": fmt.Sprintf("s3://%s/%s", r.backup.Bucket, cluster),
			"endpointURL":     r.backup.Endpoint,
			"s3Credentials": map[string]any{
				"accessKeyId":     map[string]any{"name": backupCredsName(cluster), "key": "ACCESS_KEY_ID"},
				"secretAccessKey": map[string]any{"name": backupCredsName(cluster), "key": "ACCESS_SECRET_KEY"},
			},
			"wal":  map[string]any{"compression": "gzip"},
			"data": map[string]any{"compression": "gzip"},
		},
		"retentionPolicy": "7d",
	}
}

// applyBackupCreds writes the object-store credentials secret the cluster uses.
func (r *Reconciler) applyBackupCreds(ctx context.Context, namespace, cluster string) error {
	ac := coreac.Secret(backupCredsName(cluster), namespace).
		WithStringData(map[string]string{
			"ACCESS_KEY_ID":     r.backup.AccessKey,
			"ACCESS_SECRET_KEY": r.backup.SecretKey,
		})
	if _, err := r.kube.CoreV1().Secrets(namespace).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply backup creds %s: %w", cluster, err)
	}
	return nil
}

// applyScheduledBackup ensures a daily scheduled backup exists for the cluster.
func (r *Reconciler) applyScheduledBackup(ctx context.Context, namespace, cluster string) error {
	name := cluster + "-daily"
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "ScheduledBackup",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec": map[string]any{
			"schedule":             "0 0 2 * * *", // 02:00 daily (CNPG uses 6-field cron)
			"backupOwnerReference": "self",
			"cluster":              map[string]any{"name": cluster},
		},
	}}
	if _, err := r.dyn.Resource(cnpgScheduledBackupGVR).Namespace(namespace).Apply(ctx, name, obj, applyOpts()); err != nil {
		return fmt.Errorf("apply scheduled backup %s: %w", cluster, err)
	}
	return nil
}

// TriggerBackup creates an on-demand Backup for a cluster.
func (r *Reconciler) TriggerBackup(ctx context.Context, namespace, cluster, name string) error {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "postgresql.cnpg.io/v1",
		"kind":       "Backup",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       map[string]any{"cluster": map[string]any{"name": cluster}},
	}}
	if _, err := r.dyn.Resource(cnpgBackupGVR).Namespace(namespace).
		Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create backup %s: %w", name, err)
	}
	return nil
}

// BackupInfo summarizes a Backup resource.
type BackupInfo struct {
	Name      string `json:"name"`
	Phase     string `json:"phase"`
	StartedAt string `json:"started_at,omitempty"`
	StoppedAt string `json:"stopped_at,omitempty"`
}

// backupInfos extracts BackupInfo for a cluster from an unstructured list.
func backupInfos(list *unstructured.UnstructuredList, cluster string) []BackupInfo {
	var out []BackupInfo
	for _, item := range list.Items {
		name, _, _ := unstructured.NestedString(item.Object, "spec", "cluster", "name")
		if name != cluster {
			continue
		}
		phase, _, _ := unstructured.NestedString(item.Object, "status", "phase")
		started, _, _ := unstructured.NestedString(item.Object, "status", "startedAt")
		stopped, _, _ := unstructured.NestedString(item.Object, "status", "stoppedAt")
		out = append(out, BackupInfo{Name: item.GetName(), Phase: phase, StartedAt: started, StoppedAt: stopped})
	}
	return out
}
