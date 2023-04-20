package inject

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()

	// (https://github.com/kubernetes/kubernetes/issues/57982)
	// defaulter = runtime.ObjectDefaulter(runtimeScheme)
)

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1.AddToScheme(runtimeScheme)
	// defaulting with webhooks:
	// https://github.com/kubernetes/kubernetes/issues/57982
	_ = v1.AddToScheme(runtimeScheme)

}

func GetIgnoredNamespaces() []string {
	return []string{
		metav1.NamespaceSystem,
		metav1.NamespacePublic,
	}
}

type WebhookServer struct {
	Server    *http.Server
	Params    WebhookServerParameters
	K8sClient kubernetes.Interface
}

// Webhook Server parameters.
type WebhookServerParameters struct {
	Port                int    // Webhook Server port
	CertFile            string // Path to the x509 certificate for https
	KeyFile             string // Path to the x509 private key matching `CertFile`
	InjectPrefix        string // Annotation prefix
	InjectName          string // Annotaton inject suffix
	InjectConfigMapName string // annotation config suffix
	SidecarDataKey      string
}

func failWithResponse(errMsg string) admissionv1.AdmissionResponse {
	return admissionv1.AdmissionResponse{
		Result: &metav1.Status{
			Message: errMsg,
		},
	}
}

// InjectorConfig are configuration values for the sidecar and configmap injector logic.
type InjectorConfig struct {
	InjectPrefix        string // Annotation prefix.
	InjectName          string // Annotaton inject suffix.
	InjectConfigMapName string // annotation config suffix.
	SidecarDataKey      string
}

func generateEnvs(cm *corev1.ConfigMap) []corev1.EnvVar {
	var envs []corev1.EnvVar

	re := regexp.MustCompile(`(?m)^[a-zA-Z_]+[a-zA-Z0-9_]*`)
	for key, value := range cm.Data {
		if re.MatchString(key) {
			envs = append(envs, corev1.EnvVar{Name: key, Value: value, ValueFrom: nil})
		}
	}

	return envs
}

func configmapSidecarNames(pod corev1.Pod, injectorConfig InjectorConfig) []string {
	injectConfig, err := getAnnotation(&pod.ObjectMeta, injectorConfig.InjectName, injectorConfig.InjectPrefix)
	if err != nil {
		log.Printf(
			"Skipping sidecar inject for %s/%s due missing annotation",
			pod.Namespace,
			metaName(&pod.ObjectMeta),
		)
		return nil
	}
	log.Printf(
		"Sidecar inject for %s/%s config %s due missing annotation",
		pod.Namespace,
		metaName(&pod.ObjectMeta),
		injectConfig,
	)
	parts := lo.Map(strings.Split(injectConfig, ","), func(part string, _ int) string {
		return strings.TrimSpace(part)
	})
	return parts
}

func (whsvr *WebhookServer) HandleAdmissionRequest(
	injectorConfig InjectorConfig,
	req *admissionv1.AdmissionRequest,
	ctx context.Context,
) admissionv1.AdmissionResponse {
	var err error
	if req == nil {
		return failWithResponse("Received empty request")
	}

	var pod corev1.Pod

	if err = json.Unmarshal(req.Object.Raw, &pod); err != nil {
		return failWithResponse(
			fmt.Sprintf("Could not unmarshal raw object: %v", err),
		)
	}

	log.Printf(
		"AdmissionRequest for Version=%s, Kind=%s, Namespace=%v PodName=%v UID=%v rfc6902PatchOperation=%v UserInfo=%v",
		req.Kind.Version,
		req.Kind.Kind,
		req.Namespace,
		metaName(&pod.ObjectMeta),
		req.UID,
		req.Operation,
		req.UserInfo,
	)
	// Determine whether to perform mutation.
	if !mutationRequired(GetIgnoredNamespaces(), &pod.ObjectMeta, injectorConfig) {
		log.Printf(
			"Skipping mutation for %s/%s due to policy check",
			req.Namespace,
			metaName(&pod.ObjectMeta),
		)

		return admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}
	if req.Operation != admissionv1.Create {
		log.Printf(
			"Skipping mutation for %s/%s due to operation check",
			req.Namespace,
			metaName(&pod.ObjectMeta),
		)
		return admissionv1.AdmissionResponse{
			Allowed: true,
		}
	}

	patchConfig := &PatchConfig{}
	var configMapName string

	configMapName, err = getAnnotation(&pod.ObjectMeta, injectorConfig.InjectConfigMapName, injectorConfig.InjectPrefix)
	if err != nil {
		log.Printf(
			"Skipping Env inject for %s/%s annotation not found",
			req.Namespace,
			metaName(&pod.ObjectMeta),
		)
	} else {
		var configmapEnv *corev1.ConfigMap

		configmapEnv, err = whsvr.K8sClient.CoreV1().
			ConfigMaps(req.Namespace).
			Get(ctx, configMapName, metav1.GetOptions{})
		if k8serrors.IsNotFound(err) {
			log.Printf(
				"ConfigMap %s for %s/%s not found",
				configMapName,
				req.Namespace,
				metaName(&pod.ObjectMeta),
			)
		} else if err != nil {
			log.Printf(
				"Error fetching ConfigMap %s for %s/%s %v",
				configMapName,
				req.Namespace,
				metaName(&pod.ObjectMeta),
				err,
			)
		} else {
			patchConfig.Envs = generateEnvs(configmapEnv)
		}
	}
	if configmapSidecarNames := configmapSidecarNames(pod, injectorConfig); configmapSidecarNames != nil {
		for _, configmapSidecarName := range configmapSidecarNames {
			var configmapSidecar *corev1.ConfigMap
			configmapSidecar, err = whsvr.K8sClient.CoreV1().
				ConfigMaps(req.Namespace).
				Get(ctx, configmapSidecarName, metav1.GetOptions{})
			if k8serrors.IsNotFound(err) {
				log.Printf(
					"ConfigMap %s for %s/%s not found",
					configmapSidecarName,
					req.Namespace,
					metaName(&pod.ObjectMeta),
				)
			} else if err != nil {
				log.Printf(
					"Error fetching ConfigMap %s for %s/%s %v",
					configmapSidecarName,
					req.Namespace,
					metaName(&pod.ObjectMeta),
					err,
				)
			} else if sidecarsStr, ok := configmapSidecar.Data[injectorConfig.SidecarDataKey]; ok {
				var sidecars []Sidecar
				if err = yaml.Unmarshal([]byte(sidecarsStr), &sidecars); err != nil {
					log.Printf(
						"Error unmarshalling %s in %s for %s/%s %v",
						injectorConfig.SidecarDataKey,
						configmapSidecarName,
						req.Namespace,
						metaName(&pod.ObjectMeta),
						err,
					)
				}
				for _, sidecar := range sidecars {
					patchConfig.InitContainers = append(patchConfig.InitContainers, sidecar.InitContainers...)
					patchConfig.Containers = append(patchConfig.Containers, sidecar.Containers...)
					patchConfig.Volumes = append(patchConfig.Volumes, sidecar.Volumes...)
					patchConfig.ImagePullSecrets = append(patchConfig.ImagePullSecrets, sidecar.ImagePullSecrets...)
					patchConfig.Annotations = MergeMaps(patchConfig.Annotations, sidecar.Annotations)
					patchConfig.Labels = MergeMaps(patchConfig.Labels, sidecar.Labels)
				}
			}
		}
	}

	patchBytes, err := createPatch(&pod, patchConfig)
	if err != nil {
		return admissionv1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	//log.Printf("AdmissionResponse: patch=%v\n", printPrettyPatch(patchBytes))
	return admissionv1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch

			return &pt
		}(),
	}
}

func (whsvr *WebhookServer) Health(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
}

// Serve method for webhook Server.
func (whsvr *WebhookServer) Serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := io.ReadAll(r.Body); err == nil {
			body = data
		}
	}

	if len(body) == 0 {
		log.Print("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Printf("Content-Type=%s, expecting application/json", contentType)
		http.Error(w, "invalid Content-Type, expecting `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	// Declare AdmissionResponse. This is the value that will be used to craft the
	// response on this handler.
	var admissionResponse admissionv1.AdmissionResponse

	// Decode AdmissionRequest from raw AdmissionReview bytes.
	admissionRequest, err := NewAdmissionRequest(body)
	if err != nil {
		log.Printf("could not decode body: %v", err)

		// Set AdmissionResponse with error message
		admissionResponse = admissionv1.AdmissionResponse{
			UID: admissionRequest.UID,
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		// Set AdmissionResponse with results from HandleAdmissionRequest.
		admissionResponse = whsvr.HandleAdmissionRequest(
			InjectorConfig{
				InjectPrefix:        whsvr.Params.InjectPrefix,
				InjectName:          whsvr.Params.InjectName,
				InjectConfigMapName: whsvr.Params.InjectConfigMapName,
				SidecarDataKey:      whsvr.Params.SidecarDataKey,
			},
			admissionRequest,
			r.Context(),
		)
	}

	// Ensure the response has the same UID as the original request (if the request field
	// was populated)
	admissionResponse.UID = admissionRequest.UID

	// Wrap AdmissonResponse in AdmissionReview, then marshal it to JSON.
	resp, err := json.Marshal(admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &admissionResponse,
	})
	if err != nil {
		log.Printf("could not encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}

	log.Printf("Ready to write response ...")
	if _, err := w.Write(resp); err != nil {
		log.Printf("could not write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

// NewAdmissionRequest parses raw bytes to create an AdmissionRequest. AdmissionRequest
// actually comes wrapped inside the bytes of an AdmissionReview.
func NewAdmissionRequest(reviewRequestBytes []byte) (*admissionv1.AdmissionRequest, error) {
	var ar admissionv1.AdmissionReview
	_, _, err := deserializer.Decode(reviewRequestBytes, nil, &ar)

	log.Printf("Received AdmissionReview, APIVersion: %s, Kind: %s\n", ar.APIVersion, ar.Kind)
	return ar.Request, err
}
