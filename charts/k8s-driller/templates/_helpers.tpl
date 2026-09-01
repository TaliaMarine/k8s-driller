{{- define "k8s-driller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "k8s-driller.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s" (include "k8s-driller.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "k8s-driller.labels" -}}
app.kubernetes.io/name: {{ include "k8s-driller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "k8s-driller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "k8s-driller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "k8s-driller.adminTokenSecretName" -}}
{{- if .Values.adminBootstrapToken.secretRef.name -}}
{{ .Values.adminBootstrapToken.secretRef.name }}
{{- else -}}
{{ include "k8s-driller.fullname" . }}-admin-token
{{- end -}}
{{- end -}}

{{- define "k8s-driller.sessionKeySecretName" -}}
{{- if .Values.sessionSigningKey.secretRef.name -}}
{{ .Values.sessionSigningKey.secretRef.name }}
{{- else -}}
{{ include "k8s-driller.fullname" . }}-session-key
{{- end -}}
{{- end -}}
