{{- define "vmcp.env" -}}
{{- $r := .root }}{{- $v := $r.Values }}{{- $c := .component }}{{- $cfg := .cfg }}
- {name: VMCP_TENANT_ID, value: {{ $v.tenantId | quote }}}
{{- if or (eq $c "control-plane") (eq $c "data-plane") (eq $c "migration") }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" (printf "database-%s" $c) "source" (index $v.database (index (dict "control-plane" "controlPlane" "data-plane" "dataPlane" "migration" "migration") $c)) "env" "VMCP_DATABASE_URL") }}
{{- end }}
{{- if or (eq $c "control-plane") (eq $c "data-plane") }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "master-key" "source" $v.credentials.masterKey "env" "VMCP_MASTER_KEY") }}
- {name: VMCP_SKIP_MIGRATIONS, value: "true"}
- {name: VMCP_DB_MAX_CONNS, value: {{ $v.runtime.pool.maxConnections | quote }}}
- {name: VMCP_DB_MIN_CONNS, value: {{ $v.runtime.pool.minConnections | quote }}}
- {name: VMCP_DB_ACQUIRE_TIMEOUT, value: {{ $v.runtime.pool.acquireTimeout | quote }}}
- {name: VMCP_EGRESS_ALLOW_CIDRS, value: {{ join "," $v.runtime.egressAllowCIDRs | quote }}}
- {name: VMCP_EGRESS_ALLOW_HOSTS, value: {{ join "," $v.runtime.egressAllowHosts | quote }}}
- {name: VMCP_TRUSTED_PROXIES, value: {{ join "," $v.runtime.trustedProxies | quote }}}
{{- end }}
{{- if eq $c "control-plane" }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "admin-token" "source" $v.credentials.adminToken "env" "VMCP_ADMIN_TOKEN") }}
- {name: VMCP_CP_ADDR, value: ":8081"}
- {name: VMCP_AUDIT_RETENTION_DAYS, value: {{ $v.runtime.auditRetentionDays | quote }}}
{{- with $v.runtime.oidc }}
{{- if .issuer }}
- {name: VMCP_OIDC_ISSUER, value: {{ .issuer | quote }}}
- {name: VMCP_OIDC_DISCOVERY_URL, value: {{ .discoveryURL | quote }}}
- {name: VMCP_OIDC_CLIENT_ID, value: {{ required "OIDC clientId is required" .clientId | quote }}}
- {name: VMCP_OIDC_REDIRECT_URL, value: {{ required "OIDC redirectURL is required" .redirectURL | quote }}}
- {name: VMCP_OIDC_GROUPS_CLAIM, value: {{ .groupsClaim | quote }}}
- {name: VMCP_OIDC_ADMIN_GROUPS, value: {{ join "," .adminGroups | quote }}}
- {name: VMCP_OIDC_EDITOR_GROUPS, value: {{ join "," .editorGroups | quote }}}
- {name: VMCP_OIDC_VIEWER_GROUPS, value: {{ join "," .viewerGroups | quote }}}
{{- if or .clientSecret.value .clientSecret.existingSecret }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "oidc-client-secret" "source" .clientSecret "env" "VMCP_OIDC_CLIENT_SECRET") }}
{{- end }}{{- end }}{{- end }}
{{- end }}
{{- if eq $c "data-plane" }}
- {name: VMCP_DP_ADDR, value: ":8080"}
- {name: VMCP_NODE_ID, valueFrom: {fieldRef: {fieldPath: metadata.name}}}
- {name: VMCP_AUDIT_MODE, value: {{ $v.runtime.auditMode | quote }}}
- {name: VMCP_AUTH_MAX_STALE, value: {{ $v.runtime.authorizationMaxStale | quote }}}
- {name: VMCP_DRAIN_TIMEOUT, value: {{ $v.runtime.drainTimeout | quote }}}
- {name: VMCP_PUBLIC_URL, value: {{ $v.runtime.publicGatewayURL | quote }}}
- {name: VMCP_ALLOWED_ORIGINS, value: {{ join "," $v.runtime.allowedOrigins | quote }}}
{{- if or $v.database.listener.value $v.database.listener.existingSecret }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "database-listener" "source" $v.database.listener "env" "VMCP_DB_LISTENER_URL") }}
{{- end }}
{{- if $v.rateLimit.enabled }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "redis-url" "source" $v.rateLimit.redisURL "env" "VMCP_REDIS_URL") }}
- {name: VMCP_REDIS_FAILURE_POLICY, value: {{ $v.rateLimit.failurePolicy | quote }}}
{{- end }}
{{- with $v.runtime.agentOIDC }}{{ if .issuer }}
- {name: VMCP_AGENT_OIDC_ISSUER, value: {{ .issuer | quote }}}
- {name: VMCP_AGENT_OIDC_DISCOVERY_URL, value: {{ .discoveryURL | quote }}}
- {name: VMCP_AGENT_OIDC_AUDIENCE, value: {{ required "agent OIDC audience is required" .audience | quote }}}
- {name: VMCP_AGENT_OIDC_ALGS, value: {{ join "," .algorithms | quote }}}
{{- end }}{{- end }}
{{- end }}
{{- if eq $c "web" }}
- {name: VMCP_CONTROL_PLANE_URL, value: {{ printf "http://%s:8081" (include "vmcp.name" (dict "root" $r "component" "control-plane")) | quote }}}
{{- end }}
{{- if hasPrefix "audit-exporter-" $c }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" (printf "database-%s" $c) "source" $cfg.database "env" "VMCP_AUDIT_EXPORT_DATABASE_URL") }}
- {name: VMCP_AUDIT_EXPORT_CONSUMER, value: {{ $cfg.consumer | quote }}}
{{- end }}
{{- if and $v.omni.enabled (or (eq $c "control-plane") (eq $c "data-plane") (eq $c "omni-mcp")) }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "omni-secret" "source" $v.omni.secret "env" "VMCP_OMNI_SECRET") }}
{{- end }}
{{- if and $v.omni.enabled (or (eq $c "data-plane") (eq $c "omni-mcp")) }}
- {name: VMCP_OMNI_ALLOW_ANONYMOUS, value: {{ $v.omni.allowAnonymous | quote }}}
{{- end }}
{{- if and $v.omni.enabled (or (eq $c "control-plane") (eq $c "omni-mcp")) }}
- {name: VMCP_OMNI_PUBLIC_URL, value: {{ default $v.runtime.publicGatewayURL $v.omni.publicURL | quote }}}
{{- end }}
{{- if and $v.omni.enabled (eq $c "control-plane") }}
- {name: VMCP_OMNI_URL, value: {{ printf "http://%s:8080" (include "vmcp.name" (dict "root" $r "component" "omni-mcp")) | quote }}}
{{- end }}
{{- if eq $c "omni-mcp" }}
- {name: VMCP_LISTEN, value: ":8080"}
- {name: VMCP_OMNI_DATA_PLANE_URL, value: {{ printf "http://%s:8080" (include "vmcp.name" (dict "root" $r "component" "data-plane")) | quote }}}
- {name: VMCP_OMNI_MIN_SIMILARITY, value: {{ $v.omni.minSimilarity | quote }}}
- {name: VMCP_OMNI_REFRESH_INTERVAL, value: {{ $v.omni.refreshInterval | quote }}}
- {name: VMCP_OMNI_RERANK_URL, value: {{ $v.omni.rerank.url | quote }}}
- {name: VMCP_OMNI_RERANK_MODEL, value: {{ $v.omni.rerank.model | quote }}}
- {name: VMCP_OMNI_RERANK_TIMEOUT, value: {{ $v.omni.rerank.timeout | quote }}}
- {name: VMCP_OMNI_RERANK_CANDIDATES, value: {{ $v.omni.rerank.candidates | quote }}}
- {name: VMCP_OMNI_RERANK_NO_MATCH_SCORE, value: {{ $v.omni.rerank.noMatchScore | quote }}}
- {name: VMCP_OMNI_RERANK_CONFIDENCE_SCORE, value: {{ $v.omni.rerank.confidenceScore | quote }}}
- {name: VMCP_OMNI_RERANK_MIN_SCORE, value: {{ $v.omni.rerank.minScore | quote }}}
- {name: VMCP_OMNI_RERANK_FAILURE_POLICY, value: {{ $v.omni.rerank.failurePolicy | quote }}}
- {name: VMCP_OMNI_RERANK_INSECURE_SKIP_VERIFY, value: {{ $v.omni.rerank.insecureSkipVerify | quote }}}
{{- if or $v.omni.rerank.key.value $v.omni.rerank.key.existingSecret }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "omni-rerank-key" "source" $v.omni.rerank.key "env" "VMCP_OMNI_RERANK_KEY") }}
{{- end }}
- {name: VMCP_OMNI_EMBEDDING_URL, value: {{ $v.omni.embeddingURL | quote }}}
- {name: VMCP_OMNI_EMBEDDING_INSECURE_SKIP_VERIFY, value: {{ $v.omni.embeddingInsecureSkipVerify | quote }}}
- {name: VMCP_OMNI_EMBEDDING_TEXT_FORMAT, value: {{ $v.omni.embeddingTextFormat | quote }}}
- {name: VMCP_OMNI_EMBEDDING_DIMENSIONS, value: {{ $v.omni.embeddingDimensions | quote }}}
- {name: VMCP_OMNI_EMBEDDING_REVISION, value: {{ $v.omni.embeddingRevision | quote }}}
- {name: VMCP_OMNI_EMBEDDING_QUERY_PREFIX, value: {{ $v.omni.embeddingQueryPrefix | quote }}}
- {name: VMCP_OMNI_EMBEDDING_DOCUMENT_PREFIX, value: {{ $v.omni.embeddingDocumentPrefix | quote }}}
- {name: VMCP_OMNI_EMBEDDING_MODEL, value: {{ $v.omni.embeddingModel | quote }}}
- {name: VMCP_AGENT_OIDC_ISSUER, value: {{ $v.runtime.agentOIDC.issuer | quote }}}
- {name: VMCP_ALLOWED_ORIGINS, value: {{ join "," $v.runtime.allowedOrigins | quote }}}
- {name: VMCP_TRUSTED_PROXIES, value: {{ join "," $v.runtime.trustedProxies | quote }}}
{{- if or $v.omni.embeddingKey.value $v.omni.embeddingKey.existingSecret }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" "omni-embedding-key" "source" $v.omni.embeddingKey "env" "VMCP_OMNI_EMBEDDING_KEY") }}
{{- end }}
{{- end }}
{{- range $name, $value := $cfg.env }}
- {name: {{ $name }}, value: {{ $value | toString | quote }}}
{{- end }}
{{- range $name, $source := $cfg.secretEnv }}
{{ include "vmcp.secretEnv" (dict "root" $r "id" (printf "%s-%s" $c ($name | lower | replace "_" "-")) "source" $source "env" $name) }}
{{- end }}
{{- end -}}
