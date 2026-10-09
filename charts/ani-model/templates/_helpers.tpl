{{- define "ani-model.fullname" -}}
{{- default "ani-model-service" .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- define "ani-model.labels" -}}
app.kubernetes.io/name: {{ include "ani-model.fullname" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
{{- end -}}
{{- define "ani-model.image" -}}
{{- $base := printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- if .Values.image.digest }}{{ printf "%s@%s" $base .Values.image.digest }}{{ else }}{{ $base }}{{ end -}}
{{- end -}}
{{- define "ani-model.importImage" -}}
{{- $base := printf "%s:%s" .Values.import.jobImage.repository .Values.import.jobImage.tag -}}
{{- if .Values.import.jobImage.digest }}{{ printf "%s@%s" $base .Values.import.jobImage.digest }}{{ else }}{{ $base }}{{ end -}}
{{- end -}}
