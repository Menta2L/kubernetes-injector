package inject

import (
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// RFC6902 JSON patches.
type rfc6902PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// RFC6902 JSON patch operations.
const (
	patchOperationAdd = "add"
)

// create mutation patch for resources.
func createPatch(
	pod *corev1.Pod,
	sidecarConfig *PatchConfig,
) ([]byte, error) {
	var patch []rfc6902PatchOperation
	if sidecarConfig.Envs != nil {
		for idx, c := range pod.Spec.Containers {
			finalEnvs := make([]corev1.EnvVar, 0)
			if c.Env != nil {
				finalEnvs = append(finalEnvs, c.Env...)
			}
			finalEnvs = append(finalEnvs, sidecarConfig.Envs...)
			sort.SliceStable(finalEnvs, func(i, j int) bool {
				return finalEnvs[i].Name < finalEnvs[j].Name
			})
			p := rfc6902PatchOperation{
				Op:    "add",
				Path:  fmt.Sprintf("/spec/containers/%d/env", idx),
				Value: finalEnvs,
			}
			patch = append(patch, p)
		}
	}
	if sidecarConfig.InitContainers != nil {
		patch = append(
			patch,
			addContainer(
				pod.Spec.InitContainers,
				sidecarConfig.InitContainers,
				"/spec/initContainers",
			)...,
		)
	}
	if sidecarConfig.Containers != nil {
		patch = append(
			patch,
			addContainer(
				pod.Spec.Containers,
				sidecarConfig.Containers,
				"/spec/containers",
			)...,
		)
	}
	if sidecarConfig.Volumes != nil {
		patch = append(
			patch,
			addVolume(
				pod.Spec.Volumes,
				sidecarConfig.Volumes, "/spec/volumes",
			)...,
		)
	}
	if sidecarConfig.Annotations != nil {
		patch = append(
			patch,
			updateAnnotation(
				pod.Annotations,
				sidecarConfig.Annotations,
			)...,
		)
	}
	if sidecarConfig.Labels != nil {
		patch = append(
			patch,
			updateLabels(
				pod.Labels,
				sidecarConfig.Labels,
			)...,
		)
	}
	if sidecarConfig.ContainerVolumeMounts != nil {
		patch = append(
			patch,
			addVolumeMounts(
				pod.Spec.Containers,
				sidecarConfig.ContainerVolumeMounts,
				"/spec/containers",
			)...,
		)
	}

	return json.Marshal(patch)
}

// addContainer create a patch for adding containers.
func addContainer(
	target, added []corev1.Container,
	basePath string,
) []rfc6902PatchOperation {
	first := len(target) == 0
	var (
		value interface{}
	)
	patch := make([]rfc6902PatchOperation, 0)
	for _, add := range added {
		value = add
		path := basePath
		if first {
			first = false
			value = []corev1.Container{add}
		} else {
			path += "/-"
		}
		patch = append(patch, rfc6902PatchOperation{
			Op:    patchOperationAdd,
			Path:  path,
			Value: value,
		})
	}

	return patch
}

// addVolumeMounts creates a patch for adding volume mounts.
func addVolumeMounts(
	target []corev1.Container,
	added ContainerVolumeMounts,
	basePath string,
) []rfc6902PatchOperation {
	var patch []rfc6902PatchOperation
	for index, container := range target {
		volumeMounts, ok := added[container.Name]
		if !ok || len(volumeMounts) == 0 {
			continue
		}

		if len(container.VolumeMounts) == 0 {
			volumeMount := volumeMounts[0]
			volumeMounts = volumeMounts[1:]

			path := fmt.Sprintf("%s/%d/volumeMounts", basePath, index)
			patch = append(patch, rfc6902PatchOperation{
				Op:    patchOperationAdd,
				Path:  path,
				Value: []corev1.VolumeMount{volumeMount},
			})
		}

		path := fmt.Sprintf("%s/%d/volumeMounts/-", basePath, index)
		for _, volumeMount := range volumeMounts {
			patch = append(patch, rfc6902PatchOperation{
				Op:    patchOperationAdd,
				Path:  path,
				Value: volumeMount,
			})
		}
	}

	return patch
}

// addVolume creates a patch for adding volumes.
func addVolume(
	target, added []corev1.Volume,
	basePath string,
) []rfc6902PatchOperation {
	first := len(target) == 0
	var (
		value interface{}
	)
	patch := make([]rfc6902PatchOperation, 0)
	for _, add := range added {
		value = add
		path := basePath

		if first {
			first = false
			value = []corev1.Volume{add}
		} else {
			path += "/-"
		}

		patch = append(patch, rfc6902PatchOperation{
			Op:    patchOperationAdd,
			Path:  path,
			Value: value,
		})
	}

	return patch
}

// updateAnnotation creates a patch for adding/updating annotations.
func updateAnnotation(target, added map[string]string) []rfc6902PatchOperation {
	var patch []rfc6902PatchOperation

	for key, value := range added {
		target[key] = value
	}

	path := "/metadata/annotations"
	patch = append(patch, rfc6902PatchOperation{
		Op:    patchOperationAdd,
		Path:  path,
		Value: target,
	})
	return patch
}

// updateAnnotation creates a patch for adding/updating annotations.
func updateLabels(target, added map[string]string) []rfc6902PatchOperation {
	var patch []rfc6902PatchOperation

	for key, value := range added {
		target[key] = value
	}

	path := "/metadata/labels"
	patch = append(patch, rfc6902PatchOperation{
		Op:    patchOperationAdd,
		Path:  path,
		Value: target,
	})
	return patch
}
