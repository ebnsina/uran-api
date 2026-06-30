package k8s

import (
	"context"
	"fmt"
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
)

// ServiceSpec is the desired state for one service's workload.
type ServiceSpec struct {
	Namespace string            // target namespace (created if absent)
	Name      string            // DNS-1123 workload name (service slug)
	Image     string            // image reference to run
	Port      int32             // port the container listens on (exposed as PORT env)
	Env       map[string]string // user-defined environment variables
}

// envSecretName is the Secret that holds a service's environment variables.
func envSecretName(name string) string { return name + "-env" }

// rolloutTimeout bounds how long Apply waits for a Deployment to become ready.
const rolloutTimeout = 3 * time.Minute

var ingressRouteGVR = schema.GroupVersionResource{
	Group:    "traefik.io",
	Version:  "v1alpha1",
	Resource: "ingressroutes",
}

// Host returns the external hostname for a service spec.
func (r *Reconciler) Host(spec ServiceSpec) string {
	return fmt.Sprintf("%s.%s", spec.Name, r.baseDomain)
}

// Apply reconciles the namespace, Deployment, Service, and IngressRoute for a
// spec, then waits for the Deployment rollout to complete.
func (r *Reconciler) Apply(ctx context.Context, spec ServiceSpec) error {
	if err := r.ensureNamespace(ctx, spec.Namespace); err != nil {
		return err
	}
	if err := r.applyEnvSecret(ctx, spec); err != nil {
		return err
	}
	if err := r.applyDeployment(ctx, spec); err != nil {
		return err
	}
	if err := r.applyService(ctx, spec); err != nil {
		return err
	}
	if err := r.applyIngressRoute(ctx, spec); err != nil {
		return err
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
	ac := appsac.Deployment(spec.Name, spec.Namespace).
		WithLabels(labels).
		WithSpec(appsac.DeploymentSpec().
			WithReplicas(1).
			WithSelector(metaac.LabelSelector().WithMatchLabels(labels)).
			WithTemplate(coreac.PodTemplateSpec().
				WithLabels(labels).
				WithSpec(coreac.PodSpec().
					WithContainers(coreac.Container().
						WithName("app").
						WithImage(spec.Image).
						WithPorts(coreac.ContainerPort().WithContainerPort(spec.Port)).
						WithEnv(coreac.EnvVar().WithName("PORT").WithValue(fmt.Sprintf("%d", spec.Port))).
						WithEnvFrom(coreac.EnvFromSource().WithSecretRef(
							coreac.SecretEnvSource().WithName(envSecretName(spec.Name)),
						)),
					),
				),
			),
		)
	_, err := r.kube.AppsV1().Deployments(spec.Namespace).Apply(ctx, ac, applyOpts())
	if err != nil {
		return fmt.Errorf("apply deployment %s: %w", spec.Name, err)
	}
	return nil
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

func (r *Reconciler) applyIngressRoute(ctx context.Context, spec ServiceSpec) error {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "traefik.io/v1alpha1",
		"kind":       "IngressRoute",
		"metadata": map[string]any{
			"name":      spec.Name,
			"namespace": spec.Namespace,
		},
		"spec": map[string]any{
			"entryPoints": []any{"web"},
			"routes": []any{
				map[string]any{
					"match": fmt.Sprintf("Host(`%s`)", r.Host(spec)),
					"kind":  "Rule",
					"services": []any{
						map[string]any{"name": spec.Name, "port": int64(80)},
					},
				},
			},
		},
	}}
	_, err := r.dyn.Resource(ingressRouteGVR).Namespace(spec.Namespace).
		Apply(ctx, spec.Name, obj, applyOpts())
	if err != nil {
		return fmt.Errorf("apply ingressroute %s: %w", spec.Name, err)
	}
	return nil
}

// Delete removes the Deployment, Service, IngressRoute, and env Secret for a
// named workload. Missing objects are ignored so teardown is idempotent.
func (r *Reconciler) Delete(ctx context.Context, namespace, name string) error {
	delOpts := metav1.DeleteOptions{}

	if err := r.kube.AppsV1().Deployments(namespace).Delete(ctx, name, delOpts); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete deployment %s: %w", name, err)
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
