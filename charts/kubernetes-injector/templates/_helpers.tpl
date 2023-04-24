{{/*
Expand the name of the chart.
*/}}
{{- define "kubernetes-injector.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Generate certificates when the secret doesn't exist
*/}}
{{- define "webhook.gen-certs" -}}
{{- $certs := lookup "v1" "Secret" .Release.Namespace ( printf "%s-certs" (include "common.names.name" .) ) -}}
{{- if $certs -}}
tls.crt: {{ index $certs.data "tls.crt" }}
tls.key: {{ index $certs.data "tls.key" }}
ca.crt: {{ index $certs.data "ca.crt" }}
{{- else -}}
{{- $altNames := printf "%s.%s.svc" (include "common.names.name" .) .Release.Namespace }}
{{- $ca := .ca | default (genCA  ( printf "%s-ca" (include "common.names.name" .) ) 3650) -}}
{{- $_ := set . "ca" $ca -}}
{{- $cert := genSignedCert (include "common.names.name" .) nil (list $altNames) 3650 $ca -}}
tls.crt: {{ $cert.Cert | toString | b64enc }}
tls.key: {{ $cert.Key | toString | b64enc }}
ca.crt: {{ $ca.Cert | toString | b64enc }}
{{- end -}}
{{- end -}}

{{/*
Return the proper MySQL image name
*/}}
{{- define "webhook.image" -}}
{{- include "common.images.image" (dict "imageRoot" .Values.image "global" .Values.global) }}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "kubernetes-injector.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "kubernetes-injector.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kubernetes-injector.labels" -}}
helm.sh/chart: {{ include "kubernetes-injector.chart" . }}
{{ include "kubernetes-injector.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kubernetes-injector.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubernetes-injector.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "kubernetes-injector.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kubernetes-injector.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
