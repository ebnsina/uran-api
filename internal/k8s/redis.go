package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsac "k8s.io/client-go/applyconfigurations/apps/v1"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
	metaac "k8s.io/client-go/applyconfigurations/meta/v1"

	"github.com/ebnsina/uran-api/internal/instance"
)

const redisImage = "redis:7-alpine"
const redisPort = 6379

// ProvisionRedis runs a single-instance Redis (Deployment + Service) in the
// namespace. Data is in-memory (no persistence) for now.
func (r *Reconciler) ProvisionRedis(ctx context.Context, namespace, name string) error {
	if err := r.ensureNamespace(ctx, namespace); err != nil {
		return err
	}
	if err := r.applyNamespacePolicies(ctx, namespace); err != nil {
		return err
	}
	labels := selectorLabels(name)

	dep := appsac.Deployment(name, namespace).WithLabels(labels).
		WithSpec(appsac.DeploymentSpec().
			WithReplicas(1).
			WithSelector(metaac.LabelSelector().WithMatchLabels(labels)).
			WithTemplate(coreac.PodTemplateSpec().WithLabels(labels).
				WithSpec(coreac.PodSpec().WithContainers(coreac.Container().
					WithName("redis").
					WithImage(redisImage).
					WithPorts(coreac.ContainerPort().WithContainerPort(redisPort)).
					WithResources(containerResources(instance.Small)),
				))))
	if _, err := r.kube.AppsV1().Deployments(namespace).Apply(ctx, dep, applyOpts()); err != nil {
		return fmt.Errorf("apply redis deployment %s: %w", name, err)
	}

	svc := coreac.Service(name, namespace).WithLabels(labels).
		WithSpec(coreac.ServiceSpec().
			WithSelector(labels).
			WithPorts(coreac.ServicePort().WithPort(redisPort).WithTargetPort(intstr.FromInt32(redisPort))))
	if _, err := r.kube.CoreV1().Services(namespace).Apply(ctx, svc, applyOpts()); err != nil {
		return fmt.Errorf("apply redis service %s: %w", name, err)
	}
	return nil
}

// WaitRedisReady waits for the Redis deployment to become available.
func (r *Reconciler) WaitRedisReady(ctx context.Context, namespace, name string) error {
	return r.waitForRollout(ctx, namespace, name)
}

// RedisConnectionURI returns the in-namespace connection string.
func (r *Reconciler) RedisConnectionURI(namespace, name string) string {
	return fmt.Sprintf("redis://%s.%s:%d", name, namespace, redisPort)
}

// DeleteRedis removes the Redis deployment and service.
func (r *Reconciler) DeleteRedis(ctx context.Context, namespace, name string) error {
	if err := r.kube.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{}); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete redis deployment %s: %w", name, err)
	}
	if err := r.kube.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{}); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete redis service %s: %w", name, err)
	}
	return nil
}
