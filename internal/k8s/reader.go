package k8s

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// Reader provides read-only cluster access for observability (runtime logs and
// resource metrics). It is used by the API, which should not mutate the cluster.
type Reader struct {
	kube    kubernetes.Interface
	metrics metricsclient.Interface
	dyn     dynamic.Interface
}

// NewReader builds a Reader from a kubeconfig file.
func NewReader(kubeconfigPath string) (*Reader, error) {
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig %q: %w", kubeconfigPath, err)
	}
	kube, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build kubernetes client: %w", err)
	}
	metrics, err := metricsclient.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build metrics client: %w", err)
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("build dynamic client: %w", err)
	}
	return &Reader{kube: kube, metrics: metrics, dyn: dyn}, nil
}

// ListBackups returns the CNPG backups for a cluster (read-only).
func (rd *Reader) ListBackups(ctx context.Context, namespace, cluster string) ([]BackupInfo, error) {
	list, err := rd.dyn.Resource(cnpgBackupGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	return backupInfos(list, cluster), nil
}

// PodMetric is the resource usage of one pod.
type PodMetric struct {
	Pod           string `json:"pod"`
	CPUMillicores int64  `json:"cpu_millicores"`
	MemoryBytes   int64  `json:"memory_bytes"`
}

// Metrics returns current CPU/memory usage for pods matching the selector.
func (r *Reader) Metrics(ctx context.Context, namespace, selector string) ([]PodMetric, error) {
	list, err := r.metrics.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("list pod metrics: %w", err)
	}
	out := make([]PodMetric, 0, len(list.Items))
	for _, pm := range list.Items {
		m := PodMetric{Pod: pm.Name}
		for _, c := range pm.Containers {
			m.CPUMillicores += c.Usage.Cpu().MilliValue()
			m.MemoryBytes += c.Usage.Memory().Value()
		}
		out = append(out, m)
	}
	return out, nil
}

// StreamLogs follows the logs of the first running pod matching the selector and
// copies them to w until ctx is cancelled or the stream ends. Returns
// ErrNoPods if no running pod is found.
func (r *Reader) StreamLogs(ctx context.Context, namespace, selector string, w io.Writer) error {
	pod, err := r.firstRunningPod(ctx, namespace, selector)
	if err != nil {
		return err
	}
	stream, err := r.kube.CoreV1().Pods(namespace).GetLogs(pod, &corev1.PodLogOptions{
		Container: "app",
		Follow:    true,
		TailLines: ptr(int64(200)),
	}).Stream(ctx)
	if err != nil {
		return fmt.Errorf("open log stream: %w", err)
	}
	defer stream.Close()
	_, err = io.Copy(w, stream)
	return err
}

// ErrNoPods indicates no running pod matched the selector.
var ErrNoPods = fmt.Errorf("no running pod found")

func (r *Reader) firstRunningPod(ctx context.Context, namespace, selector string) (string, error) {
	pods, err := r.kube.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return p.Name, nil
		}
	}
	return "", ErrNoPods
}

func ptr[T any](v T) *T { return &v }
