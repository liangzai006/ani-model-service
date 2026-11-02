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
{{- define "ani-model.imageReference" -}}
{{- $repository := required "image repository is required" .repository -}}
{{- if .digest -}}
{{- if not (regexMatch "^sha256:[a-f0-9]{64}$" .digest) -}}
{{- fail "image digest must be sha256 followed by 64 lowercase hexadecimal characters" -}}
{{- end -}}
{{- printf "%s@%s" $repository .digest -}}
{{- else -}}
{{- printf "%s:%s" $repository (required "image tag or digest is required" .tag) -}}
{{- end -}}
{{- end -}}
{{- define "ani-model.image" -}}
{{- include "ani-model.imageReference" .Values.image -}}
{{- end -}}
{{- define "ani-model.importImage" -}}
{{- include "ani-model.imageReference" .Values.import.jobImage -}}
{{- end -}}
{{- define "ani-model.migrationImage" -}}
{{- include "ani-model.imageReference" .Values.migration.image -}}
{{- end -}}
