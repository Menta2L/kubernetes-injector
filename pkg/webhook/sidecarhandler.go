package webhook

import (
	"context"
	"fmt"
	"strings"

	"github.com/expediagroup/kubernetes-sidecar-injector/pkg/admission"
	"github.com/ghodss/yaml"
	"github.com/samber/lo"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Sidecar Kubernetes Sidecar Injector schema
type Sidecar struct {
	Name             string                        `yaml:"name"`
	InitContainers   []corev1.Container            `yaml:"initContainers"`
	Containers       []corev1.Container            `yaml:"containers"`
	Volumes          []corev1.Volume               `yaml:"volumes"`
	ImagePullSecrets []corev1.LocalObjectReference `yaml:"imagePullSecrets"`
	Annotations      map[string]string             `yaml:"annotations"`
	Labels           map[string]string             `yaml:"labels"`
}

// SidecarInjectorPatcher Sidecar Injector patcher
type SidecarInjectorPatcher struct {
	K8sClient                kubernetes.Interface
	InjectPrefix             string
	InjectName               string
	InjectConfigMapName      string
	SidecarDataKey           string
	AllowAnnotationOverrides bool
	AllowLabelOverrides      bool
}

func (patcher *SidecarInjectorPatcher) getAnnotation(metadata *metav1.ObjectMeta, key string) (string, error) {
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	key = patcher.InjectPrefix + "/" + key
	value, hasKey := annotations[key]
	if !hasKey {
		return "", fmt.Errorf("missing annotation %s", key)
	}
	return value, nil
}
func (patcher *SidecarInjectorPatcher) sideCarInjectionAnnotation() string {
	return patcher.InjectPrefix + "/" + patcher.InjectName
}

func (patcher *SidecarInjectorPatcher) configmapSidecarNames(namespace string, pod corev1.Pod) []string {
	podName := pod.GetName()
	if podName == "" {
		podName = pod.GetGenerateName()
	}
	annotations := map[string]string{}
	if pod.GetAnnotations() != nil {
		annotations = pod.GetAnnotations()
	}
	if sidecars, ok := annotations[patcher.sideCarInjectionAnnotation()]; ok {
		parts := lo.Map(strings.Split(sidecars, ","), func(part string, _ int) string {
			return strings.TrimSpace(part)
		})

		if len(parts) > 0 {
			log.Infof("sideCar injection for %v/%v: sidecars: %v", namespace, podName, sidecars)
			return parts
		}
	}
	log.Infof("Skipping mutation for [%v]. No action required", pod.GetName())
	return nil
}

func createArrayPatches[T any](newCollection []T, existingCollection []T, path string) []admission.PatchOperation {
	var patches []admission.PatchOperation
	for index, item := range newCollection {
		indexPath := path
		var value interface{}
		first := index == 0 && len(existingCollection) == 0
		if !first {
			indexPath = indexPath + "/-"
			value = item
		} else {
			value = []T{item}
		}
		patches = append(patches, admission.PatchOperation{
			Op:    "add",
			Path:  indexPath,
			Value: value,
		})
	}
	return patches
}

func createObjectPatches(newMap map[string]string, existingMap map[string]string, path string, override bool) []admission.PatchOperation {
	var patches []admission.PatchOperation
	if existingMap == nil {
		patches = append(patches, admission.PatchOperation{
			Op:    "add",
			Path:  path,
			Value: newMap,
		})
	} else {
		for key, value := range newMap {
			if _, ok := existingMap[key]; !ok || (ok && override) {
				key = escapeJSONPath(key)
				op := "add"
				if ok {
					op = "replace"
				}
				patches = append(patches, admission.PatchOperation{
					Op:    op,
					Path:  path + "/" + key,
					Value: value,
				})
			}
		}
	}
	return patches
}

// Escape keys that may contain `/`s or `~`s to have a valid patch
// Order matters here, otherwise `/` --> ~01, instead of ~1
func escapeJSONPath(k string) string {
	k = strings.ReplaceAll(k, "~", "~0")
	return strings.ReplaceAll(k, "/", "~1")
}
func createEnvPatch(path string, cm *corev1.ConfigMap) admission.PatchOperation {
	var envs []corev1.EnvVar

	for key, value := range cm.Data {
		envs = append(envs, corev1.EnvVar{Name: key, Value: value, ValueFrom: nil})
	}

	return admission.PatchOperation{
		Op:    "add",
		Path:  path,
		Value: envs,
	}
}

// PatchPodCreate Handle Pod Create Patch
func (patcher *SidecarInjectorPatcher) PatchPodCreate(ctx context.Context, namespace string, pod corev1.Pod) ([]admission.PatchOperation, error) {
	podName := pod.GetName()
	if podName == "" {
		podName = pod.GetGenerateName()
	}
	var patches []admission.PatchOperation
	configMapName, err := patcher.getAnnotation(&pod.ObjectMeta, patcher.InjectConfigMapName)
	if err != nil {
		log.Infof("ConfigMap name not found for %v/%v: sidecars: %v", namespace, podName, err)
	} else {
		log.Infof("Load configmap %v for %v/%v", configMapName, namespace, podName)
		configmapEnv, err := patcher.K8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			log.Warnf("sidecar configmap %s/%s was not found", namespace, configMapName)
		} else if err != nil {
			log.Errorf("error fetching sidecar configmap %s/%s - %v", namespace, configMapName, err)
		} else {
			log.Errorf("Configmap loaded %s/%s - %v", namespace, configMapName, configmapEnv)
			for idx, _ := range pod.Spec.Containers {
				patches = append(patches, createEnvPatch(fmt.Sprintf("/spec/containers/%d/env", idx), configmapEnv))
			}
		}
	}
	if configmapSidecarNames := patcher.configmapSidecarNames(namespace, pod); configmapSidecarNames != nil {
		for _, configmapSidecarName := range configmapSidecarNames {
			configmapSidecar, err := patcher.K8sClient.CoreV1().ConfigMaps(namespace).Get(ctx, configmapSidecarName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				log.Warnf("sidecar configmap %s/%s was not found", namespace, configmapSidecarName)
			} else if err != nil {
				log.Errorf("error fetching sidecar configmap %s/%s - %v", namespace, configmapSidecarName, err)
			} else if sidecarsStr, ok := configmapSidecar.Data[patcher.SidecarDataKey]; ok {
				var sidecars []Sidecar
				if err := yaml.Unmarshal([]byte(sidecarsStr), &sidecars); err != nil {
					log.Errorf("error unmarshalling %s from configmap %s/%s", patcher.SidecarDataKey, pod.GetNamespace(), configmapSidecarName)
				}
				if sidecars != nil {
					for _, sidecar := range sidecars {
						patches = append(patches, createArrayPatches(sidecar.InitContainers, pod.Spec.InitContainers, "/spec/initContainers")...)
						patches = append(patches, createArrayPatches(sidecar.Containers, pod.Spec.Containers, "/spec/containers")...)
						patches = append(patches, createArrayPatches(sidecar.Volumes, pod.Spec.Volumes, "/spec/volumes")...)
						patches = append(patches, createArrayPatches(sidecar.ImagePullSecrets, pod.Spec.ImagePullSecrets, "/spec/imagePullSecrets")...)
						patches = append(patches, createObjectPatches(sidecar.Annotations, pod.Annotations, "/metadata/annotations", patcher.AllowAnnotationOverrides)...)
						patches = append(patches, createObjectPatches(sidecar.Labels, pod.Labels, "/metadata/labels", patcher.AllowLabelOverrides)...)
					}
					log.Debugf("sidecar patches being applied for %v/%v: patches: %v", namespace, podName, patches)
				}
			}
		}
	}
	return patches, nil
}

/*PatchPodUpdate not supported, only support create */
func (patcher *SidecarInjectorPatcher) PatchPodUpdate(_ context.Context, _ string, _ corev1.Pod, _ corev1.Pod) ([]admission.PatchOperation, error) {
	return nil, nil
}

/*PatchPodDelete not supported, only support create */
func (patcher *SidecarInjectorPatcher) PatchPodDelete(_ context.Context, _ string, _ corev1.Pod) ([]admission.PatchOperation, error) {
	return nil, nil
}
