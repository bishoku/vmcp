{{- define "vmcp.fullname" -}}
{{- default (printf "%s-%s" .Release.Name (default .Chart.Name .Values.nameOverride)) .Values.fullnameOverride | trunc 50 | trimSuffix "-" -}}
{{- end -}}
{{- define "vmcp.name" -}}
{{- printf "%s-%s" (include "vmcp.fullname" .root) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- define "vmcp.labels" -}}
app.kubernetes.io/name: vmcp
app.kubernetes.io/instance: {{ .root.Release.Name | quote }}
app.kubernetes.io/component: {{ .component | quote }}
{{- end -}}
{{- define "vmcp.components" -}}
{{- $all := dict "control-plane" .Values.controlPlane "data-plane" .Values.dataPlane "web" .Values.web "tenant-router" .Values.tenantRouter -}}
{{- range $name, $cfg := .Values.exporters -}}
{{- $_ := set $all (printf "audit-exporter-%s" $name) $cfg -}}
{{- end -}}
{{- $out := dict -}}
{{- range $name, $cfg := $all -}}
{{- $merged := mergeOverwrite (deepCopy $.Values.workloadDefaults) (deepCopy $cfg) -}}
{{- $_ := set $out $name $merged -}}
{{- end -}}
{{- $out | toJson -}}
{{- end -}}
{{- define "vmcp.secretName" -}}
{{- printf "%s-%s" (include "vmcp.fullname" .root) .id | trunc 54 | trimSuffix "-" -}}-{{ .id | sha256sum | trunc 8 }}
{{- end -}}
{{- define "vmcp.sources" -}}
{{- $v := .Values -}}
{{- $s := dict -}}
{{- if or $v.controlPlane.enabled $v.dataPlane.enabled }}{{ $_ := set $s "master-key" $v.credentials.masterKey }}{{ end -}}
{{- if $v.controlPlane.enabled }}{{ $_ := set $s "admin-token" $v.credentials.adminToken }}{{ $_ := set $s "database-control-plane" $v.database.controlPlane }}{{ end -}}
{{- if $v.dataPlane.enabled }}{{ $_ := set $s "database-data-plane" $v.database.dataPlane }}{{ if or $v.database.listener.value $v.database.listener.existingSecret }}{{ $_ := set $s "database-listener" $v.database.listener }}{{ end }}{{ if $v.rateLimit.enabled }}{{ $_ := set $s "redis-url" $v.rateLimit.redisURL }}{{ end }}{{ end -}}
{{- if $v.migration.enabled }}{{ $_ := set $s "database-migration" $v.database.migration }}{{ end -}}
{{- if and $v.controlPlane.enabled $v.runtime.oidc.issuer (or $v.runtime.oidc.clientSecret.value $v.runtime.oidc.clientSecret.existingSecret) }}{{ $_ := set $s "oidc-client-secret" $v.runtime.oidc.clientSecret }}{{ end -}}
{{- range $name, $cfg := (include "vmcp.components" . | fromJson) -}}
{{- if $cfg.enabled -}}
{{- if hasPrefix "audit-exporter-" $name }}{{ $_ := set $s (printf "database-%s" $name) $cfg.database }}{{ end -}}
{{- range $env, $source := $cfg.secretEnv }}{{ $_ := set $s (printf "%s-%s" $name ($env | lower | replace "_" "-")) $source }}{{ end -}}
{{- end -}}
{{- end -}}
{{- if $v.migration.enabled }}{{ range $env, $source := $v.migration.secretEnv }}{{ $_ := set $s (printf "migration-%s" ($env | lower | replace "_" "-")) $source }}{{ end }}{{ end -}}
{{- $s | toJson -}}
{{- end -}}
{{- define "vmcp.secretEnv" -}}
- name: {{ .env }}
  valueFrom:
    secretKeyRef:
      name: {{ default (include "vmcp.secretName" .) .source.existingSecret | quote }}
      key: {{ default "value" .source.key | quote }}
{{- end -}}
{{- define "vmcp.serviceAccount" -}}
{{- default (include "vmcp.name" .) .cfg.serviceAccount.name -}}
{{- end -}}
