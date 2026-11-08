---
title: Migrate to v0.5
sidebar_label: Migrate to v0.5
description: Rewrite a v0.3 configuration for v0.5, where the decision model answers every routing judgment, and review what changed.
---

# Migrate to v0.5

v0.5 is one breaking release. The configuration schema is `v0.5`, the
decision model answers every routing judgment through
[System One](../system-one/overview), and every form the Router no longer
reads fails at load with one line that says what to do.
`vllm-sr config migrate` rewrites every such form; you migrate once, then
review the notes it prints. The [release note](../release-notes/decision-model-first-class)
summarizes what changed and why.

## 1. Run the migration

```bash
vllm-sr config migrate --config config.yaml
```

The migrated configuration is written to `config.migrated.yaml`, and only when
the Router's own validation accepts it. Every rewrite lives in the Router, so
the CLI, the Dashboard and `vllm-sr config validate` agree on the result, and
migrate needs a Router binary or image to ask; there is no offline mode. A
configuration in the layout before config v0.3 (top-level `vllm_endpoints` and
the like) is refused: migrate it with `vllm-sr config migrate` of vllm-sr 0.4
first, then run this one.

A form that cannot be rewritten exactly is still rewritten as far as its
meaning allows, and migrate prints what to review: thresholds to re-fit,
dropped weights, dropped example banks, and conditions it cannot split. A few
forms have no rewrite at all, such as the `image_gen` plugin or an audio
`embeddings` rule: migrate refuses the file and its message says, after
`it has no mechanical rewrite:`, what to write instead.

## 2. Read the load errors

A configuration that still uses a removed form does not load. Each error is
one line: where the form is, an error code, what was removed, and what it
becomes:

```text
<path>: <code>: <what was removed>; run `vllm-sr config migrate` (<what it becomes>)
```

| Code | Means |
| --- | --- |
| `removed_signal_type` | A signal type that no longer exists |
| `removed_algorithm_type` | An algorithm type that no longer exists |
| `removed_plugin_type` | A plugin type that no longer exists |
| `removed_field` | A field the Router no longer reads |
| `removed_section` | A whole section the Router no longer reads |
| `invalid_value` | A value that no longer means what it did, such as `version: v0.3` |

Two forms still load, with a warning, because they are renames:
`signals.decision` and `type: decision` conditions load as `systemone`
(`renamed_signal_type`), and `algorithm.type: decision` loads as the
`systemone` selector (`renamed_algorithm_type`). Migrate rewrites both.

## 3. Review what changed

### The configuration structure

The top-level `routing` block is gone: a configuration's routing lives in
`recipes`, which are flat. Model facts live on `providers.models[]`, and
default plugins in `global.plugins`.

```yaml removed
version: v0.3
routing:
  signals:
    keywords:
      - name: urgent
        operator: OR
        keywords: [urgent, asap]
```

```text
version: invalid_value: config schema v0.3 is removed; run `vllm-sr config migrate` (version: v0.5)
```

```yaml
version: v0.5
recipes:
  - name: default
    signals:
      keywords:
        - name: urgent
          operator: OR
          keywords: [urgent, asap]
```

| Before | After |
| --- | --- |
| `version: v0.3` | `version: v0.5` |
| Top-level `routing` | `recipes: [{name: default, ...}]`, with an entrypoint marked `default: true` when there are other recipes |
| `routing.modelCards` | Each card's fields on the `providers.models[]` entry of the same name |
| `recipes[].routing.<field>` | `recipes[].<field>` |
| `routing.model_bindings` | `recipes[].model_bindings` |
| `enabled` on the memory, replay and cache stores and services | A plugin entry in `global.plugins` |

### Signals

Every model-judged signal is a question to the decision model. Fields that
configured an earlier implementation are gone; thresholds are a rule's
`predicate`:

```yaml removed
recipes:
  - name: default
    signals:
      jailbreak:
        - name: attack
          threshold: 0.88
          include_history: true
```

```text
recipes[default].signals.jailbreak[attack]: removed_field: a jailbreak rule's threshold, direction and include_history are its predicate, stage and input.history, and method and patterns are removed; run `vllm-sr config migrate` (predicate: {gte: <threshold>}, stage, input.history)
```

```yaml
recipes:
  - name: default
    signals:
      jailbreak:
        - name: attack
          predicate: {gte: 0.88}
          input:
            history: true
```

`reasks` and `topic_continuity` are one signal, `turn_relation`:

```yaml removed
recipes:
  - name: default
    signals:
      reasks:
        - name: repeated
          lookback_turns: 2
```

```text
recipes[default].signals.reasks: removed_signal_type: the reask signal is a turn_relation rule; run `vllm-sr config migrate` (signals.turn_relation with labels [reask]; lookback_turns becomes input.previous_turns)
```

```yaml
recipes:
  - name: default
    signals:
      turn_relation:
        - name: repeated
          labels: [reask]
          input:
            previous_turns: 2
```

| Before | After |
| --- | --- |
| `signals.decision`, `algorithm.type: decision` | `systemone` (loads with a warning) |
| `domains[].mmlu_categories` | `domains[].labels` |
| Domain conditions that declared their own domains | One `domains` rule per domain the conditions name |
| `jailbreak` `threshold`, `direction`, `include_history` | `predicate`, `stage`, `input.history` |
| `jailbreak` `method`, `jailbreak_patterns`, `benign_patterns` | Removed: the decision model reads the request. For a deterministic hard block, write the patterns as a [`keywords`](../tutorials/signal/heuristic/keyword) rule that a blocking decision reads |
| `jailbreak` and `pii` rules under `on_error: block` | `on_unknown: match` on the rules |
| `safety` `threshold`, `labels`, `unsafe_labels`, `model` | `predicate`; `labels` name [harm categories](../system-one/reference#harm-categories); `deployment` |
| `safety` `hazard` | The harm categories it covered, in the rule's `labels` (mapped by hand) |
| `pii` `threshold`, `pii_types_allowed`, `include_history`, `source: tool_result` | `predicate`, `ignore_labels`, `input.history`, `input.tool_results` |
| `fact_check` rule named `no_fact_check_needed` without labels | `labels: [NO_FACT_CHECK_NEEDED]` with a predicate |
| `user_feedbacks` and `modality` rules whose name chose the option | The option in `labels` |
| `reasks`, `topic_continuity` | `turn_relation` with `labels` |
| `complexity` example banks and thresholds; `name: difficulty:hard` conditions | `complexity`; `{name: difficulty, label: hard}` |
| `modality` names `AR`, `DIFFUSION`, `BOTH`; the keyword fallback | `labels` `text`, `image`, `text_image`; the fallback becomes a `keywords` rule (review) |
| `classifiers`, `preferences`, text `embeddings` rules | [`systemone` questions](../system-one/custom-questions#questions-that-replace-removed-signals) |
| Image `embeddings` rules with text anchors | `systemone` questions with `input.image` |
| `kb`; image-anchor and audio `embeddings` rules | No rewrite: write a `systemone` question (the error says which) |
| `events` | `keywords` rules with `operator: OR` |
| `structure` | `keywords` rules with `method: regex` and a `count` or `density` predicate; a `text_bytes` rule becomes a `context` rule |
| `actions` | The `systemone` choice question `action` |
| `conversation` `image_content` | An `input_modality` rule with `modality: image` |
| `global.model_catalog.modules.*` and their system lines | Removed; a module's threshold becomes the `predicate` of the rules that used it |

### Algorithms

```yaml removed
providers:
  models:
    - name: small
      backend_refs: [{name: small, endpoint: "127.0.0.1:8000", protocol: http}]
    - name: large
      backend_refs: [{name: large, endpoint: "127.0.0.1:8001", protocol: http}]
recipes:
  - name: default
    decisions:
      - name: verified
        priority: 100
        rules: {operator: AND, conditions: []}
        modelRefs: [{model: small}, {model: large}]
        algorithm:
          type: confidence
          confidence:
            confidence_method: avg_logprob
            threshold: 0.7
```

```text
recipes[default].decisions[verified].algorithm: removed_algorithm_type: algorithm type confidence is removed; run `vllm-sr config migrate` (type: escalate, with its method and threshold under escalate.accept)
```

```yaml
providers:
  models:
    - name: small
      backend_refs: [{name: small, endpoint: "127.0.0.1:8000", protocol: http}]
    - name: large
      backend_refs: [{name: large, endpoint: "127.0.0.1:8001", protocol: http}]
recipes:
  - name: default
    decisions:
      - name: verified
        priority: 100
        rules: {operator: AND, conditions: []}
        modelRefs: [{model: small}, {model: large}]
        algorithm:
          type: escalate
          escalate:
            accept:
              confidence:
                method: avg_logprob
                gte: 0.7
```

| Before | After |
| --- | --- |
| `prompt`, `router_dc` | `systemone`, which asks the decision model with the same instructions or the candidates' descriptions |
| `knn`, `kmeans`, `svm`, `mlp` | `systemone`, or `static` for a decision with one candidate; the trained model is not used |
| `automix`, `hybrid`, `latency_aware` | `multi_factor`, with weights migrate writes out |
| `confidence` (Looper) | `escalate` with `escalate.accept.confidence` |
| `cascade` (System One) | `escalate` with `escalate.stages` |
| The `automix_entailment` verifier | `escalate.accept.systemone`, the `answer_check` question |
| `algorithm.<executor>.on_error` | `algorithm.on_error`, which every executor reads |
| `algorithm.on_error` on a selector | Removed: a failed selector falls back to the first candidate |
| `global.router.model_selection.ml` | Removed with the trained selectors |

### Plugins

| Before | After |
| --- | --- |
| `tool_selection`, `global.integrations.tools` | The `select` block of the [`tools`](../tutorials/plugin/tools) plugin, with `tools_db_path` and `match.judge` |
| `tools` fields at the top of its configuration (`mode`, `allow_tools`, ...) | Under `policy` |
| `rag`, `memory`, `context_compression` | Sources, `compress` and `budget` of the [`context`](../tutorials/plugin/context) plugin |
| RAG `top_k`, `similarity_threshold`, `rerank`; memory `retrieval_limit`; compression `scoring` | `match.recall` and `match.judge` |
| RAG `min_confidence_threshold` | A `fact_check` condition in the decision's `rules` |
| The `rag.reranker` binding and `provider: rerank` deployments | Removed: the `relevance` judge on the decision model |
| `hallucination`, `response_jailbreak` | [`response_guard`](../tutorials/plugin/response-guard) rules over response-stage signals |
| `semantic-cache`, `semantic_cache`, `response-cache`; flat cache fields | [`response_cache`](../tutorials/plugin/response-cache) with `semantic.match` |
| `global.stores.semantic_cache` | `global.stores.response_cache` |
| `shadow_dispatch` | The `shadow` block of [`router_replay`](../tutorials/plugin/router-replay) |
| `emits[].retention` | The decision's [`retention`](../tutorials/decision/retention) attribute |
| `masking` `threshold` | `masking.signal`, naming a `pii` rule whose `predicate` applies |
| `system_prompt` `mode: append` | `mode: replace`, which is what it did |

### Embeddings

`embeddings.default` names its deployment in a mapping, the way
`system.decision_model` does:

```yaml removed
global:
  model_catalog:
    deployments:
      remote-embed:
        provider: openai_embeddings
        endpoint: https://embed.example.com/v1
        model: BAAI/bge-m3
        capabilities: {dimensions: 1024, modalities: [text]}
    embeddings:
      default: remote-embed
```

```text
global.model_catalog.embeddings.default: invalid_value: embeddings.default "remote-embed" is a bare deployment name; run `vllm-sr config migrate` (embeddings.default: {deployment: remote-embed}, as system.decision_model names its deployment)
```

```yaml
global:
  model_catalog:
    deployments:
      remote-embed:
        provider: openai_embeddings
        endpoint: https://embed.example.com/v1
        model: BAAI/bge-m3
        capabilities: {dimensions: 1024, modalities: [text]}
    embeddings:
      default: {deployment: remote-embed}
```

| Before | After |
| --- | --- |
| A bare `embeddings.default: <name>` | `embeddings.default: {deployment: <name>}` |
| Embedding model keywords (`mmbert`, `qwen3`, ...) and the deployments they implied | Deployment names: `global.model_catalog.embeddings.default`, or a store's `embedding` block |
| `embeddings.semantic`, `model_bindings.embedding` | `embeddings.default` |
| `embeddings.multimodal` | Removed: embeddings serve text recall; a decision about an image asks a `systemone` question with `input.image` |
| `global.services.api.embeddings` and the Router's embeddings and similarity endpoints | Removed: the embedding deployment starts when a recall feature uses it |
| A remote embedding's width and inputs | `capabilities.dimensions` and `capabilities.modalities` on its `openai_embeddings` deployment |
| A store's `embedding_model`, `vector_store.embedding_dimension` | The store's `embedding: {deployment, dimension}` |
| Admission by embedding model keyword | The admission of the embedding deployment, by its name |

### Model resources and connections

The model-resource forms of the previous release no longer load. A binding
names its deployment, and the signal decides what the deployment answers:

```yaml removed
recipes:
  - name: default
    model_bindings:
      pii: {deployment: vela, adapter: auto, mapping_path: /models/pii/labels.json}
global:
  model_catalog:
    deployments:
      vela:
        provider: model_runtime
        artifact: vllm-sr/vela3-fast
        device: cpu
```

```text
recipes[default].model_bindings.pii: removed_field: recipes[default].model_bindings.pii.adapter and mapping_path are removed; run `vllm-sr config migrate` (pii: vela; the signal decides what the deployment answers)
```

```yaml
recipes:
  - name: default
    model_bindings:
      pii: vela
global:
  model_catalog:
    deployments:
      vela:
        provider: model_runtime
        artifact: vllm-sr/vela3-fast
        device: cpu
```

A backend connects the way a deployment does, with `endpoint`, `auth` and
`headers`:

```yaml removed
providers:
  models:
    - name: claude
      backend_refs:
        - provider: anthropic
          base_url: https://gateway.example.com/anthropic
          api_key_env: ANTHROPIC_KEY
          auth_header: x-api-key
          auth_prefix: ""
          extra_headers: {X-Org: acme}
```

```text
providers.models[claude].backend_refs[0]: removed_field: providers.models[claude].backend_refs[0].base_url, api_key_env, auth_header, auth_prefix and extra_headers are removed; run `vllm-sr config migrate` (endpoint (a full URL), auth {header, prefix, api_key_env} and headers, as a deployment connects)
```

```yaml
providers:
  models:
    - name: claude
      backend_refs:
        - provider: anthropic
          endpoint: https://gateway.example.com/anthropic
          auth: {header: x-api-key, prefix: "", api_key_env: ANTHROPIC_KEY}
          headers: {X-Org: acme}
```

| Before | After |
| --- | --- |
| A binding object: `contract`, `adapter`, `head`, `mapping_path`, `operating_point` | The deployment name, `pii: vela` |
| A binding with a classifier contract (`label_distribution.v1`, `label_scores.v1`, `label_decision.v1`, `score.v1`, `token_spans.v1`) | Dropped: the decision model answers the signal |
| A binding for one rule, `safety.<rule>` or `complexity.<rule>` | The rule's `deployment` |
| A deployment's `question_types` | `capabilities.question_types` |
| `served_name` on a deployment or a replica | `model` |
| A `provider: classify` or `http` deployment | Dropped: no signal asks a classifier service |
| `provider: candle`, `ort` or `openvino` | `provider: model_runtime`, on its device, with its artifact's Hub repository; see [Migrate to the model runtime](../model-runtime/migrate) for `precision`, `profile` and the other engine fields |
| `global.model_catalog.external[]` | Removed; the `memory_rewrite` model becomes an `openai_chat` deployment that `global.stores.memory.query_rewrite` names, and its access key an environment variable |
| A backend's `base_url`, `api_key_env`, `auth_header`, `auth_prefix`, `extra_headers` | `endpoint` (a full URL), `auth: {header, prefix, api_key_env}`, `headers` |
| `global.router.auto_model_names`, `include_config_models_in_list` | The `model_names` of the default recipe's entrypoint |
| `global.integrations.looper.endpoint` | Removed: the Router makes a Looper's model calls in process |
| `global.integrations.looper.<algorithm>.model_names` | No rewrite: an entrypoint whose `model_names` name a recipe with a decision that runs the algorithm |
| `use_nli` and the NLI explainer fields | Removed: the rule's spans stay in the response |

A backend's inline `api_key` still loads; it cannot be combined with
`auth.api_key_env`. [Migrate model resources](./model-resource-migration)
shows each form in full.

### Models and their output

A model's `modality` keyword becomes the facts it stood for, and a decision
that answers with a generated image declares it in an
[output contract](../tutorials/decision/output):

```yaml removed
providers:
  models:
    - name: painter
      api_format: images
      backend_refs: [{name: images, endpoint: "127.0.0.1:8002", protocol: http}]
      modality: diffusion
```

```text
providers.models[painter].modality: removed_field: modality: diffusion is removed; run `vllm-sr config migrate` (capabilities: [image_generation] and modalities: {input: [text], output: [image]})
```

```yaml
providers:
  models:
    - name: painter
      api_format: images
      backend_refs: [{name: images, endpoint: "127.0.0.1:8002", protocol: http}]
      capabilities: [image_generation]
      modalities: {input: [text], output: [image]}
```

| Before | After |
| --- | --- |
| `modality: ar` | Removed: a model answers in text unless its `modalities` say otherwise |
| `modality: diffusion` | `capabilities: [image_generation]` and `modalities: {input: [text], output: [image]}` |
| `modality: omni` | Explicit `modalities`: `input: [text, image]`, and `output: [text, image]` when the model declares `image_generation` |
| A decision that sent image requests to a model by its modality | The decision's `output: {kind: image, action: generate}`, or `{kind: [text, image]}` for one model that writes both |
| A text-and-image route over two models | No rewrite: route images to an image model in a decision of their own |
| The `image_gen` plugin | No rewrite: an image model (`api_format: images`, `capabilities: [image_generation]`) the decision references, with `output: {kind: image, action: generate}` |
| An `embeddings` rule with an audio query | No rewrite: drop it; no decision model reads audio yet |

### Headers and gateways

Response headers are not configuration, so migrate does not rewrite what
reads them. Update gateways and clients:

| Before | After |
| --- | --- |
| A custom Envoy route on `x-selected-model` | `x-vsr-route-model` |
| `x-disable-router-memory` | `x-vsr-memory: off` |
| `x-vsr-matched-*`, decision, confidence and protocol headers | The members of `x-vsr-trace`, for a caller `global.router.headers.debug` allows |
| `x-vsr-cache-hit`, `x-vsr-fast-response` | `x-vsr-response-path` |
| `x-vsr-protocol-warnings`, `x-vsr-response-warnings` | `x-vsr-warnings` |
| `x-vsr-selected-modality` | The trace's `modality` member |
| `x-session-id`, `x-claude-code-session-id` | `x-vsr-session-id`; `global.router.headers.session_id_headers` keeps the others accepted |

The [VSR routing headers](../troubleshooting/vsr-headers) page lists the
public set and every trace member.

## 4. Validate and start

```bash
vllm-sr config validate --config config.migrated.yaml
vllm-sr serve --config config.migrated.yaml
```

## Behavior to expect

- **Thresholds were re-fitted** where the old value belonged to another
  model. A rule without a `predicate` takes the answering model's calibrated
  threshold; a model without one uses 0.5, with an `uncalibrated_preset`
  warning.
- **Span-needing rules must resolve to a span-capable deployment.** `pii`,
  `hallucination`, `span` questions and masking fail to load with
  `span_required` on a deployment without spans, such as a d3 model.
- **A `jailbreak` or `pii` rule whose model gives no answer does not match**,
  unless it sets `on_unknown: match`.
- **RAG relevance and tool selection are judged by the decision model**, and
  semantic cache hits are confirmed by its `same_request` question.
- **The embedding model starts only when a recall feature needs it**, and the
  Router's embeddings API is gone.
- **Every judge has a `max_candidates` cap**; more candidates are truncated,
  never an error.
- **System One batch calls use the `states` form only**, and a Router needs
  a model runtime of contract 4.0.0; see
  [the batch request](../system-one/api#the-batch-request).
- **Image decisions declare an `output` contract**, and every model they name
  declares the capabilities it needs. `POST /v1/images/generations` reaches
  only such a decision; anything else there gets `image_route_required`.
- **Response headers are a public set**, with debug detail in one
  `x-vsr-trace` header; the [release note](../release-notes/decision-model-first-class#headers)
  maps every old header to its replacement.
