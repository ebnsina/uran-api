package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsac "k8s.io/client-go/applyconfigurations/apps/v1"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
	metaac "k8s.io/client-go/applyconfigurations/meta/v1"

	"github.com/ebnsina/uran-api/internal/svctype"
)

// ServiceSpec is the desired state for one service's workload.
type ServiceSpec struct {
	Namespace    string            // target namespace (created if absent)
	Name         string            // DNS-1123 workload name (service slug)
	Type         string            // svctype.{Web,Static,Worker,Cron}
	Image        string            // image reference to run
	Port         int32             // port the container listens on (exposed as PORT env)
	Schedule     string            // cron expression (cron type only)
	Env          map[string]string // user-defined environment variables
	Domains      []string          // additional custom hostnames routed to this workload
	Replicas     int32             // fixed replica count (used when not autoscaling)
	InstanceSize string            // instance.* size name
	HealthPath   string            // HTTP health-check path; empty means a TCP check
	MinReplicas  int32             // autoscale floor (autoscaling on when MaxReplicas > 0)
	MaxReplicas  int32             // autoscale ceiling
}

// autoscaled reports whether the spec uses an HPA rather than a fixed count.
func (s ServiceSpec) autoscaled() bool { return s.MaxReplicas > 0 }

// envSecretName is the Secret that holds a service's environment variables.
func envSecretName(name string) string { return name + "-env" }

// tlsSecretName is the Secret cert-manager writes the issued certificate into.
func tlsSecretName(name string) string { return name + "-tls" }

// rolloutTimeout bounds how long Apply waits for a Deployment to become ready.
const rolloutTimeout = 3 * time.Minute

var ingressRouteGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "ingressroutes",
}

var certificateGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

// Host returns the default external hostname for a service spec.
func (r *Reconciler) Host(spec ServiceSpec) string {
	return fmt.Sprintf("%s.%s", spec.Name, r.baseDomain)
}

// hosts returns every hostname routed to the workload: the default host plus any
// custom domains.
func (r *Reconciler) hosts(spec ServiceSpec) []string {
	return append([]string{r.Host(spec)}, spec.Domains...)
}

// Apply reconciles the namespace, Deployment, Service, and IngressRoute for a
// spec, then waits for the Deployment rollout to complete.
func (r *Reconciler) Apply(ctx context.Context, spec ServiceSpec) error {
	if err := r.ensureNamespace(ctx, spec.Namespace); err != nil {
		return err
	}
	if err := r.applyNamespacePolicies(ctx, spec.Namespace); err != nil {
		return err
	}
	if err := r.applyEnvSecret(ctx, spec); err != nil {
		return err
	}

	// Cron services are a scheduled CronJob, not a long-running workload.
	if spec.Type == svctype.Cron {
		return r.applyCronJob(ctx, spec)
	}

	// All other types run a Deployment, optionally autoscaled.
	if err := r.applyDeployment(ctx, spec); err != nil {
		return err
	}
	if err := r.reconcileAutoscaler(ctx, spec); err != nil {
		return err
	}

	// Only routable types (web/static) get a Service, route, and TLS cert;
	// background workers run without inbound networking.
	if svctype.IsRoutable(spec.Type) {
		if err := r.applyCertificate(ctx, spec); err != nil {
			return err
		}
		if err := r.applyService(ctx, spec); err != nil {
			return err
		}
		if err := r.applyIngressRoute(ctx, spec); err != nil {
			return err
		}
	}
	return r.waitForRollout(ctx, spec.Namespace, spec.Name)
}

func (r *Reconciler) ensureNamespace(ctx context.Context, ns string) error {
	ac := coreac.Namespace(ns).WithLabels(map[string]string{"app.kubernetes.io/managed-by": fieldManager})
	_, err := r.kube.CoreV1().Namespaces().Apply(ctx, ac, applyOpts())
	if err != nil {
		return fmt.Errorf("apply namespace %s: %w", ns, err)
	}
	return nil
}

// applyEnvSecret stores the service's environment variables in a Secret that
// the Deployment consumes via envFrom. The Secret is always applied (even when
// empty) so the Deployment's secretRef is satisfied.
func (r *Reconciler) applyEnvSecret(ctx context.Context, spec ServiceSpec) error {
	ac := coreac.Secret(envSecretName(spec.Name), spec.Namespace).
		WithLabels(selectorLabels(spec.Name)).
		WithStringData(spec.Env)
	_, err := r.kube.CoreV1().Secrets(spec.Namespace).Apply(ctx, ac, applyOpts())
	if err != nil {
		return fmt.Errorf("apply env secret %s: %w", spec.Name, err)
	}
	return nil
}

func (r *Reconciler) applyDeployment(ctx context.Context, spec ServiceSpec) error {
	labels := selectorLabels(spec.Name)

	container := coreac.Container().
		WithName("app").
		WithImage(spec.Image).
		WithPorts(coreac.ContainerPort().WithContainerPort(spec.Port)).
		WithResources(containerResources(spec.InstanceSize)).
		WithEnv(coreac.EnvVar().WithName("PORT").WithValue(fmt.Sprintf("%d", spec.Port))).
		WithEnvFrom(coreac.EnvFromSource().WithSecretRef(
			coreac.SecretEnvSource().WithName(envSecretName(spec.Name)),
		))
	// Routable workloads get readiness + liveness probes so rollouts wait for
	// health and unhealthy pods are restarted. Background workers are skipped
	// (they have no HTTP/known port contract).
	if svctype.IsRoutable(spec.Type) {
		probe := healthProbe(spec)
		container = container.WithReadinessProbe(probe).WithLivenessProbe(probe)
	}

	depSpec := appsac.DeploymentSpec().
		WithSelector(metaac.LabelSelector().WithMatchLabels(labels)).
		WithTemplate(coreac.PodTemplateSpec().
			WithLabels(labels).
			WithSpec(coreac.PodSpec().WithContainers(container)),
		)
	// When autoscaling, the HPA owns the replica count — don't fight it.
	if !spec.autoscaled() {
		depSpec = depSpec.WithReplicas(spec.Replicas)
	}

	ac := appsac.Deployment(spec.Name, spec.Namespace).WithLabels(labels).WithSpec(depSpec)
	if _, err := r.kube.AppsV1().Deployments(spec.Namespace).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply deployment %s: %w", spec.Name, err)
	}
	return nil
}

// healthProbe builds the readiness/liveness probe for a spec: an HTTP GET when
// a health path is configured, otherwise a TCP connect to the port.
func healthProbe(spec ServiceSpec) *coreac.ProbeApplyConfiguration {
	if spec.HealthPath != "" {
		return coreac.Probe().WithHTTPGet(
			coreac.HTTPGetAction().WithPath(spec.HealthPath).WithPort(intstr.FromInt32(spec.Port)),
		)
	}
	return coreac.Probe().WithTCPSocket(
		coreac.TCPSocketAction().WithPort(intstr.FromInt32(spec.Port)),
	)
}

func (r *Reconciler) applyService(ctx context.Context, spec ServiceSpec) error {
	ac := coreac.Service(spec.Name, spec.Namespace).
		WithLabels(selectorLabels(spec.Name)).
		WithSpec(coreac.ServiceSpec().
			WithSelector(selectorLabels(spec.Name)).
			WithPorts(coreac.ServicePort().
				WithName("http").
				WithPort(80).
				WithTargetPort(intstr.FromInt32(spec.Port)),
			),
		)
	_, err := r.kube.CoreV1().Services(spec.Namespace).Apply(ctx, ac, applyOpts())
	if err != nil {
		return fmt.Errorf("apply service %s: %w", spec.Name, err)
	}
	return nil
}

// applyCertificate requests a cert-manager certificate covering every host
// routed to the workload, stored in the workload's TLS secret.
func (r *Reconciler) applyCertificate(ctx context.Context, spec ServiceSpec) error {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "Certificate",
		"metadata": map[string]any{
			"name":      spec.Name,
			"namespace": spec.Namespace,
		},
		"spec": map[string]any{
			"secretName": tlsSecretName(spec.Name),
			"dnsNames":   toAnySlice(r.hosts(spec)),
			"issuerRef": map[string]any{
				"name": r.certIssuer,
				"kind": "ClusterIssuer",
			},
		},
	}}
	_, err := r.dyn.Resource(certificateGVR).Namespace(spec.Namespace).
		Apply(ctx, spec.Name, obj, applyOpts())
	if err != nil {
		return fmt.Errorf("apply certificate %s: %w", spec.Name, err)
	}
	return nil
}

// httpRouteName is the plain-HTTP IngressRoute paired with the HTTPS one.
func httpRouteName(name string) string { return name + "-http" }

// applyIngressRoute applies two routes for the workload: an HTTPS route on the
// websecure entrypoint (terminating TLS with the issued certificate) and a
// plain HTTP route on the web entrypoint. A single IngressRoute cannot mix TLS
// and non-TLS entrypoints, so they are separate objects.
func (r *Reconciler) applyIngressRoute(ctx context.Context, spec ServiceSpec) error {
	match := hostMatch(r.hosts(spec))

	https := ingressRouteObject(spec.Name, spec.Namespace, spec.Name, match, []any{"websecure"}, tlsSecretName(spec.Name))
	if _, err := r.dyn.Resource(ingressRouteGVR).Namespace(spec.Namespace).
		Apply(ctx, spec.Name, https, applyOpts()); err != nil {
		return fmt.Errorf("apply https ingressroute %s: %w", spec.Name, err)
	}

	http := ingressRouteObject(httpRouteName(spec.Name), spec.Namespace, spec.Name, match, []any{"web"}, "")
	if _, err := r.dyn.Resource(ingressRouteGVR).Namespace(spec.Namespace).
		Apply(ctx, httpRouteName(spec.Name), http, applyOpts()); err != nil {
		return fmt.Errorf("apply http ingressroute %s: %w", spec.Name, err)
	}
	return nil
}

// ingressRouteObject builds an IngressRoute named name that routes match →
// service:80 on the given entrypoints. A non-empty tlsSecret adds a TLS block.
func ingressRouteObject(name, namespace, service, match string, entryPoints []any, tlsSecret string) *unstructured.Unstructured {
	spec := map[string]any{
		"entryPoints": entryPoints,
		"routes": []any{
			map[string]any{
				"match": match,
				"kind":  "Rule",
				"services": []any{
					map[string]any{"name": service, "port": int64(80)},
				},
			},
		},
	}
	if tlsSecret != "" {
		spec["tls"] = map[string]any{"secretName": tlsSecret}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "traefik.io/v1alpha1",
		"kind":       "IngressRoute",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": spec,
	}}
}

// hostMatch builds a Traefik rule matching any of the given hosts.
func hostMatch(hosts []string) string {
	parts := make([]string, len(hosts))
	for i, h := range hosts {
		parts[i] = fmt.Sprintf("Host(`%s`)", h)
	}
	return strings.Join(parts, " || ")
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// Delete removes the Deployment, Service, IngressRoute, and env Secret for a
// named workload. Missing objects are ignored so teardown is idempotent.
func (r *Reconciler) Delete(ctx context.Context, namespace, name string) error {
	delOpts := metav1.DeleteOptions{}

	if err := r.kube.AppsV1().Deployments(namespace).Delete(ctx, name, delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete deployment %s: %w", name, err)
	}
	if err := r.kube.BatchV1().CronJobs(namespace).Delete(ctx, name, delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete cronjob %s: %w", name, err)
	}
	if err := r.deleteAutoscaler(ctx, namespace, name); err != nil {
		return err
	}
	if err := r.kube.CoreV1().Services(namespace).Delete(ctx, name, delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete service %s: %w", name, err)
	}
	if err := r.kube.CoreV1().Secrets(namespace).Delete(ctx, envSecretName(name), delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete env secret %s: %w", name, err)
	}
	if err := r.dyn.Resource(ingressRouteGVR).Namespace(namespace).Delete(ctx, name, delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete ingressroute %s: %w", name, err)
	}
	if err := r.dyn.Resource(ingressRouteGVR).Namespace(namespace).Delete(ctx, httpRouteName(name), delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete http ingressroute %s: %w", name, err)
	}
	if err := r.dyn.Resource(certificateGVR).Namespace(namespace).Delete(ctx, name, delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete certificate %s: %w", name, err)
	}
	if err := r.kube.CoreV1().Secrets(namespace).Delete(ctx, tlsSecretName(name), delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete tls secret %s: %w", name, err)
	}
	return nil
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// waitForRollout polls until the Deployment reports all replicas available, or
// the context/timeout expires.
func (r *Reconciler) waitForRollout(ctx context.Context, ns, name string) error {
	ctx, cancel := context.WithTimeout(ctx, rolloutTimeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		dep, err := r.kube.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil && rolloutComplete(dep) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("rollout of %s/%s did not complete: %w", ns, name, ctx.Err())
		case <-ticker.C:
		}
	}
}

// rolloutComplete reports whether a Deployment has fully rolled out.
func rolloutComplete(d *appsv1.Deployment) bool {
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	return d.Generation == d.Status.ObservedGeneration &&
		d.Status.UpdatedReplicas == desired &&
		d.Status.AvailableReplicas == desired
}

func selectorLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/managed-by": fieldManager,
	}
}

func applyOpts() metav1.ApplyOptions {
	return metav1.ApplyOptions{FieldManager: fieldManager, Force: true}
}
