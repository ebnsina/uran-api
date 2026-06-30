// Package k8s reconciles a built deploy onto a Kubernetes cluster: it applies a
// Deployment, Service, and Traefik IngressRoute for a service and waits for the
// rollout to become available. It uses Server-Side Apply so repeated reconciles
// are idempotent.
package k8s

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// fieldManager identifies this controller as the owner of the fields it applies.
const fieldManager = "uran-controller"

// BackupConfig holds the S3-compatible object store used for database backups.
type BackupConfig struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
}

// Reconciler applies service workloads to a cluster.
type Reconciler struct {
	kube       kubernetes.Interface
	dyn        dynamic.Interface
	baseDomain string
	certIssuer string
	backup     BackupConfig
}

// NewReconciler builds a Reconciler from a kubeconfig file. baseDomain is the
// wildcard domain under which service routes are exposed (e.g. "uran.local").
// certIssuer is the cert-manager ClusterIssuer used to provision TLS. backup is
// the object store for managed-database backups.
func NewReconciler(kubeconfigPath, baseDomain, certIssuer string, backup BackupConfig) (*Reconciler, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	return &Reconciler{kube: kube, dyn: dyn, baseDomain: baseDomain, certIssuer: certIssuer, backup: backup}, nil
}
