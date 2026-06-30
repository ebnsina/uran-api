package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
// namespace, with a persistent volume and append-only persistence so data
// survives restarts.
func (r *Reconciler) ProvisionRedis(ctx context.Context, namespace, name, storage string) error {
	if err := r.ensureNamespace(ctx, namespace); err != nil {
		return err
	}
	if err := r.applyNamespacePolicies(ctx, namespace); err != nil {
		return err
	}
	labels := selectorLabels(name)

	// Persistent volume for the append-only file.
	pvc := coreac.PersistentVolumeClaim(pvcName(name), namespace).
		WithLabels(labels).
		WithSpec(coreac.PersistentVolumeClaimSpec().
			WithAccessModes(corev1.ReadWriteOnce).
			WithResources(coreac.VolumeResourceRequirements().
				WithRequests(corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(storage)})))
	if _, err := r.kube.CoreV1().PersistentVolumeClaims(namespace).Apply(ctx, pvc,
		metav1.ApplyOptions{FieldManager: fieldManager}); err != nil {
		return fmt.Errorf("apply redis pvc %s: %w", name, err)
	}

	res := instance.Get(instance.Small)
	container := coreac.Container().
		WithName("redis").
		WithImage(redisImage).
		WithArgs("--appendonly", "yes").
		WithPorts(coreac.ContainerPort().WithContainerPort(redisPort)).
		WithVolumeMounts(coreac.VolumeMount().WithName(diskVolumeName).WithMountPath("/data")).
		WithResources(coreac.ResourceRequirements().
			WithRequests(corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(res.CPURequest), corev1.ResourceMemory: resource.MustParse(res.MemRequest)}).
			WithLimits(corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(res.CPULimit), corev1.ResourceMemory: resource.MustParse(res.MemLimit)}))

	dep := appsac.Deployment(name, namespace).WithLabels(labels).
		WithSpec(appsac.DeploymentSpec().
			WithReplicas(1).
			// RWO volume: roll without an extra pod so the old one releases it.
			WithStrategy(appsac.DeploymentStrategy().
				WithType(appsv1.RollingUpdateDeploymentStrategyType).
				WithRollingUpdate(appsac.RollingUpdateDeployment().
					WithMaxSurge(intstr.FromInt32(0)).WithMaxUnavailable(intstr.FromInt32(1)))).
			WithSelector(metaac.LabelSelector().WithMatchLabels(labels)).
			WithTemplate(coreac.PodTemplateSpec().WithLabels(labels).
				WithSpec(coreac.PodSpec().
					WithContainers(container).
					WithVolumes(coreac.Volume().WithName(diskVolumeName).
						WithPersistentVolumeClaim(coreac.PersistentVolumeClaimVolumeSource().WithClaimName(pvcName(name)))))))
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

// DeleteRedis removes the Redis deployment, service, and volume.
func (r *Reconciler) DeleteRedis(ctx context.Context, namespace, name string) error {
	if err := r.kube.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{}); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete redis deployment %s: %w", name, err)
	}
	if err := r.kube.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{}); ignoreNotFound(err) != nil {
		return fmt.Errorf("delete redis service %s: %w", name, err)
	}
	if err := r.deletePVC(ctx, namespace, pvcName(name)); err != nil {
		return err
	}
	return nil
}
