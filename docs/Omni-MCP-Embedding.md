# Embedding models for Omni MCP

Omni supports a configured **OpenAI-compatible text embeddings HTTP API**. It
ships no embedding model, model weights, vector database or implicit provider.
A model name alone is insufficient: run the model behind a compatible API, then
configure the complete endpoint and the model identifier that endpoint accepts.

The API contract and limits below are based on the current Omni implementation.
Basic embeddings and optional provider authentication are available in v1.2.0;
the insecure TLS flag requires v1.2.1. The detailed diagnostic logs described in
the [installation guide](Omni-MCP-Installation.md#embedding-diagnostics-in-pod-logs)
require v1.2.2 or later.

## Deployment choices

| Choice | Omni configuration | Requirements |
|---|---|---|
| Keyword search only | Leave `VMCP_OMNI_EMBEDDING_URL` unset | No model service, model or provider key needed; index mode is `keyword` |
| On-prem API without authentication | Set URL and model; leave KEY unset or empty | Reachable compatible endpoint accepting unauthenticated requests |
| On-prem API with a bearer token | Set URL, model and KEY | Service accepts `Authorization: Bearer <key>` |
| Hosted API with a bearer token | Set URL, model and KEY | Approved provider, compatible response, appropriate quota/network access |
| Provider with a different API/auth scheme | Put a compatible adapter/proxy in front | Adapter implements the contract below and handles provider-specific authentication/options |

**An API key is optional.** With an empty `VMCP_OMNI_EMBEDDING_KEY`, Omni sends no
Authorization header at all. A dummy key is unnecessary. This provider credential
is separate from an agent runtime key and from the internal `VMCP_OMNI_SECRET`,
which is still required for communication between gateway components.

Omni supports a bearer key, not arbitrary authentication headers, API-key header
names, provider OAuth token acquisition or client-certificate configuration.
Configure a compatible intermediary if your model service requires those features.
A native model endpoint that returns just an `embeddings` array, or a chat/completion
endpoint, is not compatible simply because it can run an embedding model.

## Required request and response contract

Set a full URL such as `https://embeddings.internal/v1/embeddings`. Omni uses the
URL verbatim; it does **not** append `/v1/embeddings`. The model name is also sent
verbatim and must match a deployed model/alias at that endpoint. There is no model
allowlist, model auto-download or automatic fallback to a different model.

Requests use `POST`, `Content-Type: application/json`, and this body:

```json
{
  "model": "your-deployed-embedding-model",
  "input": ["Customer support find_orders Find customer orders", "Order delivery status"]
}
```

If KEY is nonempty, Omni additionally sends `Authorization: Bearer <key>`; supply
the key value without a `Bearer ` prefix. The provider must return **HTTP 200** and
one JSON numeric vector per input. This deliberately small example uses three
dimensions; actual vectors may have any supported length:

```json
{
  "data": [
    {"index": 0, "embedding": [0.12, -0.34, 0.56]},
    {"index": 1, "embedding": [0.23, -0.45, 0.67]}
  ]
}
```

Entries may arrive out of order: Omni uses their `index` to restore input order.
Each index must occur exactly once, from zero through input count minus one.
Each vector must be nonempty, finite, and have the same length as other vectors
in that response. Additional provider fields such as `usage` are ignored.
Base64 vectors, sparse vectors, token arrays and streaming responses are unsupported.
HTTP redirects are not followed, including login redirects and URL canonicalization
redirects; configure the final API URL directly.

The default `legacy-v1` representation concatenates server name, server
description, tool name and tool description. The discovery-quality update offers
an opt-in `VMCP_OMNI_EMBEDDING_TEXT_FORMAT=tool-first-v1`: `Tool`, `Purpose`,
`Service`, `Context`, in that order. In this format names are bounded to 128
characters, purpose to 1,800 and service context to 320, with whitespace/punctuation
boundaries preferred near the limit. It is an evaluated candidate, not a new
quality-guaranteed default. Set Helm `omni.embeddingTextFormat` accordingly.
Both representations use **one document/vector per tool**, not overlapping chunks.
The 32-item embedding batch contains separate tools and is not a chunk size.
Character bounds are not tokenizer-aware token limits. Search embeds the user's intent. Tool arguments, full tool
schemas and execution results are not embedding inputs. Tool metadata and search
queries therefore leave Omni for the configured model service.

## Vector size, similarity and model selection

| Property | Current behavior |
|---|---|
| Accepted vector dimension | **1–4,096 inclusive**; common sizes such as 384, 768, 1,024, 1,536 and 3,072 fit this limit |
| Storage | Dense in-memory `float32` vectors, one per published tool entry |
| Similarity | Cosine similarity combined with exact-token keyword matching |
| Search score | Legacy lexical/cosine scoring remains the baseline; field-weighted lexical scoring and RRF are compared separately before promotion |
| Output dimension selection | Provider determines output. `VMCP_OMNI_EMBEDDING_DIMENSIONS` asserts its expected size (0 = learn); it does **not** send a `dimensions` request field or resize vectors |
| Numeric encoding | JSON numbers; Omni decodes to float32, independent of the provider's internal model precision |
| Normalization | Pre-normalized vectors are not required; cosine handles magnitudes. Zero vectors are rejected by the discovery-quality update |
| Model scope | One configured model/endpoint per Omni process; no per-project or per-vMCP model selection |

Choose a model whose output is within the accepted dimension range and whose
semantic space works for both tool descriptions and user intents. A generative
chat model, reranker or sparse-only embedding model cannot replace this API.
For multilingual discovery, evaluate the actual languages and vocabulary of your
organization; vector size alone does not establish relevance quality.

Omni sends the basic `model`/`input` contract for indexing and search. The
discovery-quality update adds separate optional string prefixes:

```env
VMCP_OMNI_EMBEDDING_QUERY_PREFIX="query: "
VMCP_OMNI_EMBEDDING_DOCUMENT_PREFIX="passage: "
VMCP_OMNI_EMBEDDING_DIMENSIONS=768
VMCP_OMNI_EMBEDDING_REVISION=model-weights-v2
```

Use the exact prefixes required by your chosen model; **do not enable these example
prefixes for all models**. Quotes above are shell/env-file syntax and are not part
of the prefix. Preserve intentional trailing spaces. For Docker `--env-file`, use
literal unquoted values and verify how your environment-file loader preserves
whitespace; Helm string values preserve it explicitly. Native provider task-type
fields, query/document model names, tokenizer options and dimension selection are
not sent. Configure those at the provider/adapter when needed.

If your provider supports reduced dimensions, select output dimensions on that
server/alias or adapter. Set `VMCP_OMNI_EMBEDDING_DIMENSIONS` to the resulting size
to **validate** it, not request reduction. Default 0 learns the first valid index
batch's dimension. Subsequent batches and queries must match; inconsistency falls
back visibly. The maximum remains 4,096; Omni does not truncate vectors to fit.

**Keep model/version and dimension identical for indexing and queries.** The
profile fingerprints endpoint, model, declared revision, prefixes, expected size,
selected text format and text representation version. Refresh reuses vectors only from the same
profile. A changed profile rebuilds vectors; profile drift or a missing vector in
the authorized candidate set selects consistent lexical `hybrid-degraded` mode.
A failed/invalid query vector returns `keyword-fallback`. Status exposes
`embeddedDocuments`, `dimensions` and the profile fingerprint; it does not expose
credentials or raw vectors.

A same-size replacement behind an unchanged provider alias cannot be detected
from numbers alone. Bump `VMCP_OMNI_EMBEDDING_REVISION` and restart all replicas
when changing provider model weights/version. There is no automatic model revision
discovery. These profile/coverage/prefix features require the discovery-quality
update in v1.2.2; v1.2.1 does not recognize the new variables.

The v1.2.0 development acceptance used
`sentence-transformers/paraphrase-multilingual-mpnet-base-v2` behind the repository's
OpenAI-compatible test fixture. This is evidence for that synthetic test, not a
bundled production server or a general model certification. See the
[acceptance report](../benchmark/results/omni-1000-100-20261027.md) and
[reproduction instructions](../benchmark/omni/README.md#real-semantic-model-and-relevance).
Any other model must meet the same API contract and be evaluated for your workload.

## Fixed operational limits

These are implementation limits, not configurable environment defaults:

| Limit | Value and consequence |
|---|---|
| Index embedding batch | Up to **32 texts/request**, sent sequentially within one refresh; provider must support array input and a full batch |
| Search embedding batch | **1 text/request**, on every semantic search; no query embedding cache |
| Text length per embedding | First **12,000 Unicode characters** per input; truncation is by characters, not tokens |
| Public discovery intent length | At most **4,096 bytes** before embedding |
| HTTP request timeout | **15 seconds** including connection/response reading, or an earlier caller cancellation/deadline |
| Successful response decoding | At most **8 MiB** of response body is read for JSON decoding |
| Index size | At most **100,000 published tool documents** per Omni index |
| Retries | No application-level embedding retry/backoff loop; later catalog refreshes retry missing vectors |
| Search admission | Default 120 searches/minute per credential per replica; 64 concurrent public MCP requests per Omni process |

For the anonymous-access update, search quota is per verified IP when no runtime
credential is provided. These admission limits are separate from provider limits.
A provider can reject a 32-item batch because of aggregate token limits even if
individual strings fit its context window. Omni has no tokenizer-aware splitting
or automatic smaller-batch retry. A 12,000-character input may exceed a model's
token limit. Keep descriptions concise and size the provider for this contract.

`VMCP_OMNI_REFRESH_INTERVAL` controls how often catalog refresh is requested; it
is not the HTTP timeout or a guarantee that a complete index builds within that
interval. For 1,000 new tools, indexing requires 32 provider calls. At 100 semantic
searches/s the provider also receives roughly 100 single-input requests/s, plus
indexing work. Every Omni replica builds its own index and generates its own
provider traffic. Startup/model cold-loading must fit the 15-second request budget;
preload/warm the model service if necessary.

Raw vector storage is approximately `documents × dimensions × 4` bytes: 1,000 tools
at 768 dimensions need about **2.93 MiB** for vector elements, versus **15.625 MiB**
at 4,096 dimensions. These are lower bounds, not pod RSS budgets: metadata, Go
allocations, temporary JSON/batches, concurrent queries and refresh overlap add
memory. Model inference CPU/GPU/RAM belongs to the separate provider service and
is not included in Omni pod usage.

## Configure an on-prem model without an API key

Put these literal values in the Omni environment file, or export them when running
a native process. Replace the URL and model alias with your own service settings:

```env
VMCP_OMNI_EMBEDDING_URL=http://embeddings.internal:8080/v1/embeddings
VMCP_OMNI_EMBEDDING_MODEL=your-deployed-embedding-model
VMCP_OMNI_EMBEDDING_KEY=
VMCP_OMNI_EMBEDDING_INSECURE_SKIP_VERIFY=false
```

There is no auth-related restriction on this setup. HTTP is supported and sends
plaintext traffic; use an appropriately isolated network or HTTPS. For an HTTPS
service, the certificate must be trusted by Omni's runtime trust store. If you
explicitly choose to skip certificate/hostname verification, set
`VMCP_OMNI_EMBEDDING_INSECURE_SKIP_VERIFY=true` (v1.2.1+). That flag affects HTTPS
verification only; it neither enables HTTP nor bypasses provider authentication.

If a previous deployment used a provider key, remove/empty the variable and any
Helm Secret reference rather than merely leaving an old key in the pod. A nonempty
stale key will still be sent and can cause an otherwise unauthenticated provider
or proxy to reject the request.

### Docker Compose

```sh
export VMCP_OMNI_EMBEDDING_URL=http://embeddings.internal:8080/v1/embeddings
export VMCP_OMNI_EMBEDDING_MODEL=your-deployed-embedding-model
export VMCP_OMNI_EMBEDDING_KEY=
# Keep your existing bridge/master/admin secrets.
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.omni.yml up -d --build
```

The overlay passes the variables through to Omni. The hostname must resolve and
the port must be reachable **from the Omni container**, not just your laptop.
The development overlay builds the checked-out source; for a published deployment,
use the corresponding versioned image and deployment procedure.

### Helm without a provider key

Merge into your existing production values; keep the existing bridge secret and
digest-pinned image overlay:

```yaml
omni:
  enabled: true
  embeddingURL: http://embeddings.model-serving.svc.cluster.local:8080/v1/embeddings
  embeddingModel: your-deployed-embedding-model
  embeddingKey:
    existingSecret: ""
    key: value
    value: ""
  embeddingInsecureSkipVerify: false
```

With no key value and no existing Secret reference, the chart adds no embedding
key environment variable. Ensure no manual `omni.env` override supplies one. If
network policies are enabled, permit Omni egress to the model service namespace,
pods and port (or the external endpoint); the embedding URL does not automatically
add a network-policy exception.

### Bearer-protected provider

Keep the same URL/model configuration and set:

```env
VMCP_OMNI_EMBEDDING_KEY=<provider-key-without-Bearer-prefix>
```

For Helm, prefer `omni.embeddingKey.existingSecret` and its `key` field instead of
putting the secret value in a committed values file. This is the only difference
needed for a compatible bearer-protected endpoint.

## Verify compatibility before connecting Omni

From the Omni pod's network context or an equivalent diagnostic container, test
one input without authentication:

```sh
curl --fail-with-body --max-time 15 \
  http://embeddings.internal:8080/v1/embeddings \
  -H 'Content-Type: application/json' \
  --data '{"model":"your-deployed-embedding-model","input":["Find customer orders"]}'
```

For a bearer-protected provider, add its Authorization header. For HTTPS, test
with normal certificate validation first. Then test **32 inputs**, including
realistic description lengths, rather than assuming a single successful input
establishes batch compatibility. Check that every `data[].embedding` is a JSON
numeric array of a stable dimension within 1–4,096 and `data[].index` covers all
inputs. Confirm the result completes within 15 seconds and fits the decode limit.

After changing environment variables, restart/recreate Omni; they are read at
startup. Confirm publication and approved tools, wait for indexing, and run
`discover_tools`. `/readyz` means a catalog is available, not that every vector
exists or that semantic relevance is correct. Inspect the index's `mode`/`error`
and each search's `search_mode`, plus the
[embedding pod diagnostics](Omni-MCP-Installation.md#embedding-diagnostics-in-pod-logs).

For HTTP 400, verify the deployed model name, array-input support and provider
context/batch limits. For 401/403 check whether the service expects no auth or a
bearer key. For 404 verify both the endpoint path and model alias; 429 indicates
provider quota/concurrency pressure. Compare logs with the provider's own logs
when it omits useful machine-readable error identifiers.

## Change a model, version or vector dimension

1. Pin the provider's model/version and output dimension. Avoid replacing the
   model behind an unchanged endpoint/alias while Omni is serving requests.
2. Update Omni's URL/model/key as needed and restart **every Omni replica**. Even
   if the alias/env values stay the same, restart when the provider model changes.
3. Let each replica rebuild its in-memory index from the current published catalog.
   The quality update fingerprints the embedding profile, so profile changes cannot
   reuse old vectors. A provider-only model change under an unchanged alias still
   requires a revision bump/restart; **Refresh index** alone is incremental.
4. Verify complete indexing, dimension consistency, search mode and a representative
   set of institution-specific intents before resuming your desired search load.

Embedding failures preserve keyword search; missing index vectors are retried on
later refreshes. Failed query embeddings are not automatically retried within the
search request. These failures do not disable normal direct vMCP calls. A successful query embedding alone
does not prove that all catalog entries have compatible vectors. Keep the provider
contract stable for the lifetime of an index.

## Metadata and discovery evaluation

Write concise tool purposes that distinguish action, object, preconditions and
returned information. Prefer `Read fulfillment status of an existing order` over
`Process order request`. Keep shared server context short. Explain acronyms and
business vocabulary used by your agents; avoid unrelated keyword lists. Descriptions
are searchable untrusted data, not executable agent instructions.

The lexical stage recognizes underscore and camel-case identifiers. Field-weighted
lexical scoring and RRF remain evaluation candidates until their measured quality
justifies promotion. Discovery remains filtered
by fresh data-plane authorization before ranking and again before result delivery.
One tool still has one vector; multi-chunk retrieval is not introduced. The next
optional stage adds [external reranking and no-match calibration](Omni-MCP-Reranking.md). A nonempty result is not a guarantee of relevance.

Compare model/text/ranking changes with labeled intents, near-neighbor wrong tools,
out-of-catalog intents, language slices and ACL cases. Report Hit@1, Hit@5, MRR,
negative-query false positives and latency separately. The repository's expanded
[quality corpus](../benchmark/omni/quality-cases.json) is synthetic development data,
not an institution-specific held-out production benchmark. See the
[reproduction instructions](../benchmark/omni/README.md#discovery-quality-comparison).

### Optional relevance floor

The discovery-quality update adds `VMCP_OMNI_MIN_SIMILARITY` (Helm
`omni.minSimilarity`), a cosine floor between 0 and 1. Default 0 preserves baseline
behavior. With complete compatible vectors and a successful query embedding,
candidates below the floor are omitted even if they match keywords. Results may
therefore be empty. The value is not a probability or universal confidence score;
calibrate it per model/language on both relevant and out-of-catalog cases.

The floor is not applied to keyword-only or embedding-fallback searches: those
have no comparable cosine score. Check `search_mode`; a floor cannot guarantee
no false positives during a provider outage. The quality comparison's fixed 0.35
experiment is diagnostic, not a recommended production value or a tuned release
default. Empty/ambiguous results should lead the agent to clarify or refine its
intent rather than guess an executable tool.

The first [measured quality comparison](../benchmark/results/omni-discovery-quality-20261027.md)
retained legacy defaults: the structured/RRF candidates did not consistently improve
per-language results. The fixed floor experiment removed negatives but lost correct
matches. Review this evidence before opting into a new text format or threshold.
