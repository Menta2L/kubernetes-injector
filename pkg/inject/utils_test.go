package inject

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"text/template"

	jsonpatch "github.com/evanphx/json-patch"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// applyPatchToAdmissionRequest runs an AdmissionRequest (wrapped in an AdmissionReview)
// through the sidecar-injector logic to extract a mutation patch, it then applies this
// patch to the origin Pod template spec to return the mutated Pod template spec.
func applyPatchToAdmissionRequest(reviewRequestBytes []byte) ([]byte, error) {
	req, err := NewAdmissionRequest(reviewRequestBytes)
	if err != nil {
		return nil, err
	}
	cm := configMap("dummy", "test-config")
	icm := invalidConfigMap("dummy", "invalid-test-config")
	scm := sidecarconfigMap("dummy", "sidecar-config")
	client := fake.NewSimpleClientset(&cm, &scm, &icm)

	whsvr := &WebhookServer{
		K8sClient: client,
	}
	admissionRes := whsvr.HandleAdmissionRequest(
		InjectorConfig{
			InjectPrefix:        "injector.server-lab.info",
			InjectName:          "inject",
			InjectConfigMapName: "config",
			SidecarDataKey:      "sidecars.yaml",
		},
		req,
		context.Background(),
	)

	patch, err := jsonpatch.DecodePatch(admissionRes.Patch)
	if err != nil {
		return nil, err
	}

	return patch.Apply(req.Object.Raw)
}
func sendAdmissionRequest(reviewRequestBytes []byte) (admissionv1.AdmissionResponse, error) {
	req, err := NewAdmissionRequest(reviewRequestBytes)
	if err != nil {
		return admissionv1.AdmissionResponse{}, err
	}
	cm := configMap("dummy", "test-config")

	client := fake.NewSimpleClientset(&cm)

	whsvr := &WebhookServer{
		K8sClient: client,
	}
	return whsvr.HandleAdmissionRequest(
		InjectorConfig{
			InjectPrefix:        "injector.server-lab.info",
			InjectName:          "inject",
			InjectConfigMapName: "config",
			SidecarDataKey:      "sidecars.yaml",
		},
		req,
		context.Background(),
	), nil
}

func configMap(namespace, name string) v1.ConfigMap {
	return v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: map[string]string{
			"TEST1": "value-1",
			"TEST2": "value-2",
			"TEST3": "value-3",
		},
	}
}
func invalidConfigMap(namespace, name string) v1.ConfigMap {
	return v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Data: map[string]string{
			"1TEST1": "value-1",
			"TEST2":  "value-2",
			"TEST3":  "value-3",
		},
	}
}
func sidecarconfigMap(namespace, name string) v1.ConfigMap {

	// Create resource object
	return v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"sidecars.yaml": "- name: haystack-agent\n  containers:\n    - name: haystack-agent\n      image: expediadotcom/haystack-agent\n      imagePullPolicy: IfNotPresent\n      args:\n        - --config-provider\n        - file\n        - --file-path\n        - /app/haystack/agent.conf\n      volumeMounts:\n        - name: agent-conf\n          mountPath: /app/haystack\n  volumes:\n    - name: agent-conf\n      configMap:\n        name: haystack-agent-conf-configmap\n  annotations:\n    my: annotation\n  labels:\n    my: label\n",
		},
	}
}

// newTestAdmissionRequest creates an Admission Request (wrapped in a Admission Review).
// This is done by embedding a pod template spec, whose path is an argument, inside the
// shell of an example Admission Request. This method simplifies generating test
// Admission Requests.
func newTestAdmissionRequest(podTemplateSpecPath string) ([]byte, error) {
	t := template.Must(
		template.ParseFiles(
			"./testdata/authenticator-admission-request.tmpl.json",
		),
	)

	pod, err := os.ReadFile(podTemplateSpecPath)
	if err != nil {
		return nil, err
	}

	var reqJSON bytes.Buffer

	err = t.Execute(&reqJSON, string(pod))
	if err != nil {
		return nil, err
	}

	var reqPrettyJSON bytes.Buffer
	err = json.Indent(&reqPrettyJSON, reqJSON.Bytes(), "", "  ")
	if err != nil {
		return nil, err
	}

	return reqPrettyJSON.Bytes(), nil
}
