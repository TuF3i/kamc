{{/* 名称与标签：与 Helm 官方脚手架保持一致 */}}
{{- define "kamc-platform.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kamc-platform.fullname" -}}
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

{{- define "kamc-platform.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kamc-platform.labels" -}}
helm.sh/chart: {{ include "kamc-platform.chart" . }}
{{ include "kamc-platform.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.podLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{- define "kamc-platform.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kamc-platform.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "kamc-platform.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kamc-platform.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "kamc-platform.secretName" -}}
{{- default (printf "%s-secrets" (include "kamc-platform.fullname" .)) .Values.secrets.existingSecret }}
{{- end }}

{{- define "kamc-platform.jwtSecretName" -}}
{{- default (printf "%s-jwt" (include "kamc-platform.fullname" .)) .Values.jwtKeyring.existingSecret }}
{{- end }}

{{- define "kamc-platform.externalURL" -}}
{{- required "config.externalURL 必填（kubeconfig/安装清单的对外地址）" .Values.config.externalURL }}
{{- end }}

{{- define "kamc-platform.jwtIssuer" -}}
{{- default (include "kamc-platform.externalURL" .) .Values.config.jwtIssuer }}
{{- end }}

{{- define "kamc-platform.oidcRedirectURL" -}}
{{- default (printf "%s/auth/callback" (include "kamc-platform.externalURL" .)) .Values.oidc.redirectURL }}
{{- end }}

{{- define "kamc-platform.avatarClaimName" -}}
{{- default (printf "%s-avatar" (include "kamc-platform.fullname" .)) .Values.avatar.persistence.existingClaim }}
{{- end }}

{{/* 头像存储后端：auto 时配置了 MinIO endpoint 即 minio，否则 local */}}
{{- define "kamc-platform.avatarBackend" -}}
{{- $backend := default "auto" .Values.avatar.backend -}}
{{- if eq $backend "auto" -}}
{{- if .Values.avatar.minio.endpoint -}}minio{{- else -}}local{{- end -}}
{{- else -}}
{{- $backend -}}
{{- end -}}
{{- end }}

{{/* MinIO 凭据所在的 Secret（优先专用 existingSecret，否则平台 Secret） */}}
{{- define "kamc-platform.avatarSecretName" -}}
{{- default (include "kamc-platform.secretName" .) .Values.avatar.minio.existingSecret -}}
{{- end }}
