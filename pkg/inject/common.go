package inject

import (
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Sidecar Kubernetes Sidecar Injector schema.
type Sidecar struct {
	Name             string                        `yaml:"name"`
	InitContainers   []corev1.Container            `yaml:"initContainers"`
	Containers       []corev1.Container            `yaml:"containers"`
	Volumes          []corev1.Volume               `yaml:"volumes"`
	ImagePullSecrets []corev1.LocalObjectReference `yaml:"imagePullSecrets"`
	Annotations      map[string]string             `yaml:"annotations"`
	Labels           map[string]string             `yaml:"labels"`
}

func metaName(meta *metav1.ObjectMeta) string {
	name := meta.GenerateName
	if name == "" {
		name = meta.Name
	}

	return name
}

// mutationRequired determines if target resource requires mutation.
func mutationRequired(ignoredList []string, metadata *metav1.ObjectMeta, injectorConfig InjectorConfig) bool {
	// skip special Kubernetes system namespaces.
	for _, namespace := range ignoredList {
		if metadata.Namespace == namespace {
			log.Printf(
				"Skip mutation for %v for it' in special namespace:%v",
				metaName(metadata),
				metadata.Namespace,
			)

			return false
		}
	}

	injectValue, _ := getAnnotation(metadata, injectorConfig.InjectName, injectorConfig.InjectPrefix)
	configValue, _ := getAnnotation(metadata, injectorConfig.InjectConfigMapName, injectorConfig.InjectPrefix)
	required := false
	if injectValue != "" || configValue != "" {
		required = true
	}

	log.Printf(
		"Mutation policy for %s/%s: required:%v",
		metaName(metadata),
		metadata.Name,
		required,
	)

	return required
}

func getAnnotation(metadata *metav1.ObjectMeta, key string, prefix string) (string, error) {
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	key = prefix + "/" + key
	value, hasKey := annotations[key]

	if !hasKey {
		return "", fmt.Errorf("missing annotation %s", key)
	}
	return value, nil
}

func MergeMaps(m1 map[string]string, m2 map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range m1 {
		merged[k] = v
	}

	for key, value := range m2 {
		//Rather than replacing the existing value for the student,
		//add on to any value we already stored.
		merged[key] = merged[key] + value
	}
	return merged
}
