# Optional Omni reranking and no-match calibration

This feature is available from **v1.2.2**. Reranking is disabled unless `VMCP_OMNI_RERANK_URL` is configured. No model
weights, Python runtime or external model service are bundled into Omni.

## Search flow

1. Obtain the caller's current data-plane access manifest.
2. Retrieve a bounded set of authorized tool candidates using the existing
   embedding/keyword path and any explicit server/project filters.
3. **Refresh authorization immediately before sending tool metadata to the
   reranker.** Removed or changed tools are filtered out; auth outages fail closed.
4. Send the intent and bounded tool descriptions to the configured rerank API.
5. Apply the query-level no-match floor. In the optional uncertain region retain
   retrieval order; otherwise sort original tool entries using returned scores.
   An independent per-tool floor can further filter results. Empty results are valid.
6. For server discovery, preserve candidate diversity (one best candidate per
   selected server first, then up to three tool views per server), and collapse
   tools by server **after** reranking. Candidate pools can therefore be smaller
   than the configured target.
7. Recheck authorization before returning the final result to the caller.

Only authorized candidate metadata is sent to the rerank provider. It receives
the tool-first bounded representation (tool, purpose, service, context), not tool
schemas/results or runtime credentials. Provider-supplied metadata is ignored:
only validated scores and original candidate indices are used. Discovery scores
do not grant execution rights or replace upstream approvals and runtime policy.

Rerank cannot recover a correct tool absent from the candidate set. Candidate
recall and final ranking quality must be measured independently. A request for up
to `limit` tools may return fewer or zero. Agents should clarify/refine an intent
when no suitable tool is returned rather than assume one exists.

## Environment reference

These variables are read by the Omni process only; restart/recreate Omni after
changing them. They do not alter the data-plane or embedding client's settings.

| Variable | Default | Meaning |
|---|---|---|
| `VMCP_OMNI_RERANK_URL` | Empty, disabled | Complete rerank API URL, e.g. `http://reranker.internal:8080/v1/rerank`; no path is appended |
| `VMCP_OMNI_RERANK_MODEL` | Empty | Required when URL is set; provider-deployed model name/alias |
| `VMCP_OMNI_RERANK_KEY` | Empty | Optional provider bearer key; empty sends **no Authorization header** |
| `VMCP_OMNI_RERANK_TIMEOUT` | `5s` | Positive request duration, at most `30s`; includes response reading |
| `VMCP_OMNI_RERANK_CANDIDATES` | `20` | Retrieval pool target, 1–100; actual pool is at least the requested result limit, capped at 100 |
| `VMCP_OMNI_RERANK_NO_MATCH_SCORE` | `0` | Query-level floor: if the best candidate is below it, return no tools |
| `VMCP_OMNI_RERANK_CONFIDENCE_SCORE` | `0` | If the best score is below this but passes no-match, preserve retrieval order and report uncertainty |
| `VMCP_OMNI_RERANK_MIN_SCORE` | `0` | Optional score floor, 0–1; 0 disables filtering. This is not the embedding cosine floor |
| `VMCP_OMNI_RERANK_FAILURE_POLICY` | `fallback` | `fallback` returns retrieval results on provider failure; `closed` returns HTTP 503 |
| `VMCP_OMNI_RERANK_INSECURE_SKIP_VERIFY` | `false` | Explicit opt-in to skip certificate-chain/hostname verification for this HTTPS client only |

Empty key supports on-prem services with no API authentication. The internal
Omni bridge secret and caller's runtime credential are separate from this key.
Provide a bearer key without a `Bearer ` prefix. Arbitrary auth headers, provider
OAuth acquisition and client-certificate setup are not implemented; adapt them
behind a compatible service/proxy if needed. HTTPS uses Omni's runtime trust store
unless verification is explicitly disabled. HTTP is also supported.

The provider returns scores in **[0,1]**; raw logits are rejected rather than
silently transformed. If a model emits logits, normalize them in the provider
adapter and record that transformation in your calibration profile. A bounded
score is not automatically a calibrated probability. Thresholds are specific to
the model/version, normalization and document representation. A generic 0.5
threshold is not a product recommendation.

## API contract

Omni sends `POST`, `Content-Type: application/json`, with this request:

```json
{
  "model": "your-deployed-reranker",
  "query": "Find the current stage of my order",
  "documents": [
    "Tool: get_order_status\nPurpose: Read order fulfillment status.\nService: Orders\nContext: Customer order operations",
    "Tool: cancel_order\nPurpose: Cancel an order before shipment.\nService: Orders\nContext: Customer order operations"
  ],
  "top_n": 2
}
```

The provider must return HTTP 200 and **one result for every input candidate**:

```json
{
  "results": [
    {"index": 1, "relevance_score": 0.03},
    {"index": 0, "relevance_score": 0.94}
  ]
}
```

Order may differ; `index` restores identity. Indices must be explicit, unique and
within the original array. Scores must be explicit finite JSON numbers in [0,1].
Do not apply a provider-side score threshold or return only a short top-N subset:
Omni requests all candidate scores and filters locally. Missing, duplicate,
out-of-range or malformed results fail the operation according to the failure
policy. Decode reads at most 2 MiB. HTTP redirects are not followed and model
requests are not automatically retried.

This is an explicit rerank-compatible contract, not an OpenAI embeddings or chat
endpoint. Check compatibility rather than assuming all serving engines expose the
same rerank response. Query strings remain bounded by Omni's normal discovery
limit, 4,096 bytes; document fields are bounded by the tool-first representation.
The provider is responsible for its tokenizer, token/context budget and model
loading. Prewarm it so normal requests fit the configured timeout.

## Failure and no-match behavior

| Condition | Behavior |
|---|---|
| Rerank not configured | Existing retrieval behavior, no rerank call |
| Rerank succeeds | `search_mode` ends in `-reranked`; `score` is the provider relevance score |
| Best score below `NO_MATCH_SCORE` | Empty tools/servers list, mode ends in `-reranked-no-match` |
| Best score passes no-match but is below `CONFIDENCE_SCORE` | Preserve retrieval order/scores, mode ends in `-rerank-uncertain`; apply any explicit per-tool floor |
| All scores below independent `MIN_SCORE` | Empty list after per-tool filtering, `-reranked` mode |
| Rerank fails, `fallback` policy | Retrieval order/scores retained, mode ends in `-rerank-fallback`; rerank threshold is **not** applied to incompatible retrieval scores |
| Rerank fails, `closed` policy | HTTP 503, no retrieval results returned |
| Authorization refresh fails | Fail closed before provider dispatch, regardless of rerank policy |
| Caller cancels/deadline expires | Stop the operation, no silent fallback |

When no-match guarantees matter, explicitly configure **`closed`**. The default
fallback favors availability and can return tools that would have been removed
by a successful rerank threshold. A no-match score floor is a relevance setting,
not a security boundary. Direct tool execution still follows its own fresh
runtime authorization even if the tool was not selected by the search.

Embedding failure can still produce keyword candidates for reranking; this is
visible in the composed `search_mode`. If keyword retrieval finds no candidates,
reranking is skipped. It cannot infer missing tools or fetch unapproved tools.

## Configure a keyless on-prem service

```env
VMCP_OMNI_RERANK_URL=http://reranker.internal:8080/v1/rerank
VMCP_OMNI_RERANK_MODEL=your-deployed-reranker
VMCP_OMNI_RERANK_KEY=
VMCP_OMNI_RERANK_CANDIDATES=20
VMCP_OMNI_RERANK_TIMEOUT=5s
VMCP_OMNI_RERANK_MIN_SCORE=0
VMCP_OMNI_RERANK_NO_MATCH_SCORE=0
VMCP_OMNI_RERANK_CONFIDENCE_SCORE=0
VMCP_OMNI_RERANK_FAILURE_POLICY=closed
```

Start with floors 0 to collect evaluation scores, then choose values from labeled
calibration data and test them on a separate validation set. `noMatchScore` is a
query-level decision; `minScore` removes individual tools. `confidenceScore`
controls whether to trust the reranker order at all. These are different policies,
not interchangeable thresholds. A model-qualified
threshold example belongs in your deployment's values, not in a universal default.
Keep approved embedding settings unchanged; embedding and rerank are separate
services/contracts with separate credentials and TLS switches.

Helm values, merged into the existing production installation:

```yaml
omni:
  rerank:
    url: http://reranker.models.svc.cluster.local:8080/v1/rerank
    model: your-deployed-reranker
    key:
      existingSecret: ""
      value: ""
    candidates: 20
    timeout: 5s
    minScore: 0
    noMatchScore: 0
    confidenceScore: 0
    failurePolicy: closed
    insecureSkipVerify: false
```

For a bearer-protected service use `omni.rerank.key.existingSecret` and its `key`
field. Only Omni receives that secret. The Compose Omni overlay passes the same
variables through. Allow DNS and network-policy egress to the provider explicitly;
setting a URL does not create a Kubernetes policy exception. Operator-provided
`omni.env` overrides must not accidentally retain an old provider key.

## Calibration and evaluation

Keep retrieval model/text/candidate settings and reranker model/version/score
normalization fixed. Split related intents and their language paraphrases together
before selecting thresholds, so translated copies do not leak between partitions.
Use calibration data to select a threshold, then report untouched validation data.
The repository corpus is already known synthetic development data; partitioning it
is useful for engineering but does not turn it into an independent production
holdout. An institution-specific held-out corpus is still required.

Report at least:

- Candidate recall at the chosen pool size, before reranking.
- Hit@1, Hit@5 and MRR by language and action family after reranking.
- Out-of-catalog false-positive queries, empty results and lost correct matches.
- Provider latency, candidate count, timeout/error rate and resource budget.
- ACL revocation before/after model dispatch and failure-policy behavior.

A threshold improving negatives while losing too many correct actions must not be
promoted. Preserving retrieval order in a low-score uncertain region can protect
strong baseline matches, but it must be evaluated: a score is not a calibrated
confidence probability. More candidates can improve recall but increase provider cost. Tool
invocation TPS is not semantic-search/rerank TPS. A sequential CPU test does not
establish a 100-searches/s capacity guarantee.

## Diagnostics

Startup logs `omni reranker configuration` with enabled state, candidate count,
score floor, timeout, failure policy and TLS verification mode. Request failures
log WARN `omni dependency failed`, `operation=search_rerank`, reason/status,
candidates and elapsed milliseconds. Identical consecutive failures are summarized
per 30 seconds; a subsequent success logs recovery. Raw query/document text,
credentials and raw provider messages/bodies are omitted.

The private index status and console show whether rerank is configured and its
candidate/floor settings. `vmcp_omni_reranker_enabled` is a configuration gauge,
not a provider health probe. `/readyz` still indicates index readiness, not rerank
availability or calibrated relevance. Inspect response `search_mode` and model
logs when diagnosing fallback or timeouts.

Development measurements and limitations: [stage 2 evaluation report](../benchmark/results/omni-rerank-stage2-20261027.md).
