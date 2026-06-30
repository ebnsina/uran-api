package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
)

// pullSecretName is the per-namespace image pull secret aggregating an org's
// registry credentials.
const pullSecretName = "uran-pull"

// RegistryCred is one registry's credentials for building a pull secret.
type RegistryCred struct {
	Registry string
	Username string
	Password string
}

// reconcilePullSecret writes (or removes) the namespace's dockerconfigjson pull
// secret from the given credentials.
func (r *Reconciler) reconcilePullSecret(ctx context.Context, namespace string, creds []RegistryCred) error {
	if len(creds) == 0 {
		err := r.kube.CoreV1().Secrets(namespace).Delete(ctx, pullSecretName, metav1.DeleteOptions{})
		if ignoreNotFound(err) != nil {
			return fmt.Errorf("delete pull secret: %w", err)
		}
		return nil
	}
	dockerJSON, err := dockerConfigJSON(creds)
	if err != nil {
		return err
	}
	ac := coreac.Secret(pullSecretName, namespace).
		WithType(corev1.SecretTypeDockerConfigJson).
		WithData(map[string][]byte{corev1.DockerConfigJsonKey: dockerJSON})
	if _, err := r.kube.CoreV1().Secrets(namespace).Apply(ctx, ac, applyOpts()); err != nil {
		return fmt.Errorf("apply pull secret: %w", err)
	}
	return nil
}

// dockerConfigJSON builds the ~/.docker/config.json payload for the credentials.
func dockerConfigJSON(creds []RegistryCred) ([]byte, error) {
	type entry struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Auth     string `json:"auth"`
	}
	auths := make(map[string]entry, len(creds))
	for _, c := range creds {
		auths[c.Registry] = entry{
			Username: c.Username,
			Password: c.Password,
			Auth:     base64.StdEncoding.EncodeToString([]byte(c.Username + ":" + c.Password)),
		}
	}
	return json.Marshal(map[string]any{"auths": auths})
}
