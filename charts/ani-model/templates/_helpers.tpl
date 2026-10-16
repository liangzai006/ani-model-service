{{- define "ani-model.fullname" -}}
{{- default "ani-model-service" .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- define "ani-model.labels" -}}
{{- if ne .Release.Namespace "ani-system" -}}
{{- fail "ani-model must be installed into the ani-system namespace" -}}
{{- end -}}
app.kubernetes.io/name: {{ include "ani-model.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
{{- end -}}
{{- define "ani-model.image" -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}
{{- define "ani-model.importImage" -}}
{{- printf "%s:%s" .Values.import.jobImage.repository .Values.import.jobImage.tag -}}
{{- end -}}
{{- define "ani-model.migrationImage" -}}
{{- printf "%s:%s" .Values.migration.image.repository .Values.migration.image.tag -}}
{{- end -}}
