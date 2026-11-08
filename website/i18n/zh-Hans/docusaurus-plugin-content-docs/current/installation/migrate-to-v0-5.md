---
title: 迁移到 v0.5
sidebar_label: 迁移到 v0.5
description: 把 v0.3 配置改写为 v0.5（在 v0.5 中由决策模型回答每个路由判断），并检查有哪些变化。
is_mtpe: false
translation:
  source_commit: "118fb72eb67a0e7193697e350d7b494dc016631e"
  source_file: "docs/installation/migrate-to-v0-5.md"
  outdated: false
---

# 迁移到 v0.5 {#migrate-to-v05}

v0.5 是一次破坏性版本。配置 schema 为 `v0.5`，决策模型通过 [System One](../system-one/overview) 回答每个路由判断，Router 不再读取的每种形式都会在加载时失败，并用一行说明该怎么做。`vllm-sr config migrate` 会改写所有这类形式；你只需迁移一次，然后检查它打印的说明。[发布说明](../release-notes/decision-model-first-class)总结了变化及其原因。

## 1. 运行迁移 {#1-run-the-migration}

```bash
vllm-sr config migrate --config config.yaml
```

迁移后的配置写入 `config.migrated.yaml`，并且只有在 Router 自身的校验接受时才会写入。所有改写都在 Router 中完成，所以 CLI、Dashboard 和 `vllm-sr config validate` 得到的结果一致；migrate 需要一个可以询问的 Router 二进制或镜像，没有离线模式。采用 config v0.3 之前布局的配置（顶层的 `vllm_endpoints` 等）会被拒绝：先用 vllm-sr 0.4 的 `vllm-sr config migrate` 迁移它，再运行本版本的 migrate。

无法精确改写的形式，会在保留其含义的范围内尽量改写，并由 migrate 打印需要检查的内容：需要重新拟合的阈值、被丢弃的权重、被丢弃的示例库，以及无法拆分的条件。少数形式完全无法改写，例如 `image_gen` 插件或带音频查询的 `embeddings` 规则：migrate 会拒绝这个文件，错误信息在 `it has no mechanical rewrite:` 之后说明应该改写成什么。

## 2. 读懂加载错误 {#2-read-the-load-errors}

仍在使用已移除形式的配置无法加载。每个错误占一行：形式所在的位置、错误代码、移除了什么，以及它变成什么：

```text
<path>: <code>: <what was removed>; run `vllm-sr config migrate` (<what it becomes>)
```

| 代码 | 含义 |
| --- | --- |
| `removed_signal_type` | 已不存在的信号类型 |
| `removed_algorithm_type` | 已不存在的算法类型 |
| `removed_plugin_type` | 已不存在的插件类型 |
| `removed_field` | Router 不再读取的字段 |
| `removed_section` | Router 不再读取的整个段落 |
| `invalid_value` | 含义已经改变的值，例如 `version: v0.3` |

有两种形式因为只是改名，仍能加载并给出警告：`signals.decision` 和 `type: decision` 条件按 `systemone` 加载（`renamed_signal_type`），`algorithm.type: decision` 按 `systemone` 选择器加载（`renamed_algorithm_type`）。migrate 会改写两者。

## 3. 检查变化 {#3-review-what-changed}

### 配置结构 {#the-configuration-structure}

顶层 `routing` 块已移除：配置的路由位于扁平的 `recipes` 中。模型事实位于 `providers.models[]`，默认插件位于 `global.plugins`。

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

| 之前 | 之后 |
| --- | --- |
| `version: v0.3` | `version: v0.5` |
| 顶层 `routing` | `recipes: [{name: default, ...}]`，存在其他配方时附带一个标记为 `default: true` 的入口 |
| `routing.modelCards` | 每张卡的字段移到同名的 `providers.models[]` 条目上 |
| `recipes[].routing.<field>` | `recipes[].<field>` |
| `routing.model_bindings` | `recipes[].model_bindings` |
| memory、replay 和 cache 存储与服务上的 `enabled` | `global.plugins` 中的一个插件条目 |

### 信号 {#signals}

每个由模型判断的信号都是向决策模型提出的问题。用于配置旧实现的字段已移除；阈值是规则的 `predicate`：

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

`reasks` 和 `topic_continuity` 合并为一个信号 `turn_relation`：

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

| 之前 | 之后 |
| --- | --- |
| `signals.decision`、`algorithm.type: decision` | `systemone`（加载时给出警告） |
| `domains[].mmlu_categories` | `domains[].labels` |
| 自行声明领域的 domain 条件 | 为条件指定的每个领域各写一条 `domains` 规则 |
| `jailbreak` 的 `threshold`、`direction`、`include_history` | `predicate`、`stage`、`input.history` |
| `jailbreak` 的 `method`、`jailbreak_patterns`、`benign_patterns` | 已移除：决策模型直接读取请求。需要确定性的硬拦截时，把这些模式写成一条 [`keywords`](../tutorials/signal/heuristic/keyword) 规则，并由一个拦截决策读取它 |
| `on_error: block` 下的 `jailbreak` 和 `pii` 规则 | 规则上的 `on_unknown: match` |
| `safety` 的 `threshold`、`labels`、`unsafe_labels`、`model` | `predicate`；`labels` 指定[危害类别](../system-one/reference#harm-categories)；`deployment` |
| `safety` 的 `hazard` | 把它覆盖的危害类别写进规则的 `labels`（手动对应） |
| `pii` 的 `threshold`、`pii_types_allowed`、`include_history`、`source: tool_result` | `predicate`、`ignore_labels`、`input.history`、`input.tool_results` |
| 名为 `no_fact_check_needed` 且没有 labels 的 `fact_check` 规则 | `labels: [NO_FACT_CHECK_NEEDED]` 加一个 predicate |
| 以名称决定选项的 `user_feedbacks` 和 `modality` 规则 | 把选项写进 `labels` |
| `reasks`、`topic_continuity` | 带 `labels` 的 `turn_relation` |
| `complexity` 的示例库和阈值；`name: difficulty:hard` 条件 | `complexity`；`{name: difficulty, label: hard}` |
| `modality` 名称 `AR`、`DIFFUSION`、`BOTH`；关键词回退 | `labels` `text`、`image`、`text_image`；回退变为一条 `keywords` 规则（需检查） |
| `classifiers`、`preferences`、文本 `embeddings` 规则 | [`systemone` 问题](../system-one/custom-questions#questions-that-replace-removed-signals) |
| 带文本锚点的图片 `embeddings` 规则 | 带 `input.image` 的 `systemone` 问题 |
| `kb`；图片锚点和音频 `embeddings` 规则 | 没有改写：编写一个 `systemone` 问题（错误信息会说明） |
| `events` | `operator: OR` 的 `keywords` 规则 |
| `structure` | `method: regex` 加 `count` 或 `density` predicate 的 `keywords` 规则；`text_bytes` 规则变为 `context` 规则 |
| `actions` | `systemone` choice 问题 `action` |
| `conversation` 的 `image_content` | `modality: image` 的 `input_modality` 规则 |
| `global.model_catalog.modules.*` 及其 system 行 | 已移除；模块的阈值变为使用它的规则的 `predicate` |

### 算法 {#algorithms}

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

| 之前 | 之后 |
| --- | --- |
| `prompt`、`router_dc` | `systemone`，用同样的说明或候选描述询问决策模型 |
| `knn`、`kmeans`、`svm`、`mlp` | `systemone`，只有一个候选的决策为 `static`；不再使用训练出的模型 |
| `automix`、`hybrid`、`latency_aware` | `multi_factor`，由 migrate 写出权重 |
| `confidence`（Looper） | 带 `escalate.accept.confidence` 的 `escalate` |
| `cascade`（System One） | 带 `escalate.stages` 的 `escalate` |
| `automix_entailment` 验证器 | `escalate.accept.systemone`，即 `answer_check` 问题 |
| `algorithm.<executor>.on_error` | `algorithm.on_error`，所有执行器都读取它 |
| 选择器上的 `algorithm.on_error` | 已移除：失败的选择器回退到第一个候选 |
| `global.router.model_selection.ml` | 随训练式选择器一起移除 |

### 插件 {#plugins}

| 之前 | 之后 |
| --- | --- |
| `tool_selection`、`global.integrations.tools` | [`tools`](../tutorials/plugin/tools) 插件的 `select` 块，包含 `tools_db_path` 和 `match.judge` |
| 写在 `tools` 配置顶层的字段（`mode`、`allow_tools` 等） | 放到 `policy` 下 |
| `rag`、`memory`、`context_compression` | [`context`](../tutorials/plugin/context) 插件的源、`compress` 和 `budget` |
| RAG 的 `top_k`、`similarity_threshold`、`rerank`；记忆的 `retrieval_limit`；压缩的 `scoring` | `match.recall` 和 `match.judge` |
| RAG 的 `min_confidence_threshold` | 决策 `rules` 中的一个 `fact_check` 条件 |
| `rag.reranker` 绑定和 `provider: rerank` 部署 | 已移除：由决策模型上的 `relevance` 评审代替 |
| `hallucination`、`response_jailbreak` | 基于响应阶段信号的 [`response_guard`](../tutorials/plugin/response-guard) 规则 |
| `semantic-cache`、`semantic_cache`、`response-cache`；扁平的缓存字段 | 带 `semantic.match` 的 [`response_cache`](../tutorials/plugin/response-cache) |
| `global.stores.semantic_cache` | `global.stores.response_cache` |
| `shadow_dispatch` | [`router_replay`](../tutorials/plugin/router-replay) 的 `shadow` 块 |
| `emits[].retention` | 决策的 [`retention`](../tutorials/decision/retention) 属性 |
| `masking` 的 `threshold` | `masking.signal`，指定一条 `pii` 规则，使用它的 `predicate` |
| `system_prompt` 的 `mode: append` | `mode: replace`，这正是它原来的行为 |

### 嵌入 {#embeddings}

`embeddings.default` 用一个映射指定它的部署，写法和 `system.decision_model` 一样：

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

| 之前 | 之后 |
| --- | --- |
| 只写名称的 `embeddings.default: <name>` | `embeddings.default: {deployment: <name>}` |
| 嵌入模型关键词（`mmbert`、`qwen3` 等）以及它们隐含的部署 | 部署名称：`global.model_catalog.embeddings.default`，或存储的 `embedding` 块 |
| `embeddings.semantic`、`model_bindings.embedding` | `embeddings.default` |
| `embeddings.multimodal` | 已移除：嵌入只用于文本召回；关于图片的决策向 `systemone` 提出带 `input.image` 的问题 |
| `global.services.api.embeddings` 以及 Router 的嵌入和相似度端点 | 已移除：召回功能用到嵌入部署时它才启动 |
| 远程嵌入的向量宽度和输入 | 它的 `openai_embeddings` 部署上的 `capabilities.dimensions` 和 `capabilities.modalities` |
| 存储的 `embedding_model`、`vector_store.embedding_dimension` | 存储的 `embedding: {deployment, dimension}` |
| 按嵌入模型关键词设置的准入 | 嵌入部署的准入，按其名称设置 |

### 模型资源与连接 {#model-resources-and-connections}

上一版本的模型资源写法不再能加载。绑定只写部署名称，由信号决定该部署回答什么：

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

后端的连接方式与部署相同，使用 `endpoint`、`auth` 和 `headers`：

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

| 之前 | 之后 |
| --- | --- |
| 绑定对象：`contract`、`adapter`、`head`、`mapping_path`、`operating_point` | 部署名称，例如 `pii: vela` |
| 使用分类器契约（`label_distribution.v1`、`label_scores.v1`、`label_decision.v1`、`score.v1`、`token_spans.v1`）的绑定 | 删除：由决策模型回答该信号 |
| 针对单条规则的绑定，`safety.<rule>` 或 `complexity.<rule>` | 该规则的 `deployment` |
| 部署上的 `question_types` | `capabilities.question_types` |
| 部署或副本上的 `served_name` | `model` |
| `provider: classify` 或 `http` 的部署 | 删除：没有任何信号询问分类服务 |
| `provider: candle`、`ort` 或 `openvino` | `provider: model_runtime`，保留其设备，并使用其工件的 Hub 仓库；`precision`、`profile` 等引擎字段见[迁移到模型运行时](../model-runtime/migrate) |
| `global.model_catalog.external[]` | 已移除；`memory_rewrite` 模型改为一个由 `global.stores.memory.query_rewrite` 指定的 `openai_chat` 部署，它的访问密钥改为环境变量 |
| 后端的 `base_url`、`api_key_env`、`auth_header`、`auth_prefix`、`extra_headers` | `endpoint`（完整 URL）、`auth: {header, prefix, api_key_env}`、`headers` |
| `global.router.auto_model_names`、`include_config_models_in_list` | 默认配方入口的 `model_names` |
| `global.integrations.looper.endpoint` | 已移除：Router 在进程内发起 Looper 的模型调用 |
| `global.integrations.looper.<algorithm>.model_names` | 无法改写：一个入口，它的 `model_names` 指向一个配方，该配方中有运行该算法的决策 |
| `use_nli` 及 NLI 解释器字段 | 已移除：规则的 span 仍保留在响应中 |

后端内联的 `api_key` 仍可加载，但不能与 `auth.api_key_env` 同时使用。[迁移模型资源](./model-resource-migration)完整展示了每种写法。

### 模型及其输出 {#models-and-their-output}

模型的 `modality` 关键词改写成它所代表的事实；以生成图片作答的决策在[输出契约](../tutorials/decision/output)中声明这一点：

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

| 之前 | 之后 |
| --- | --- |
| `modality: ar` | 已移除：除非 `modalities` 另有说明，模型都以文本作答 |
| `modality: diffusion` | `capabilities: [image_generation]` 和 `modalities: {input: [text], output: [image]}` |
| `modality: omni` | 显式的 `modalities`：`input: [text, image]`；模型声明了 `image_generation` 时 `output: [text, image]` |
| 按模型的 modality 把图片请求发给某个模型的决策 | 决策的 `output: {kind: image, action: generate}`；一个模型同时写文本和图片时为 `{kind: [text, image]}` |
| 跨两个模型的文本加图片路由 | 无法改写：把图片请求路由到单独决策中的图片模型 |
| `image_gen` 插件 | 无法改写：决策引用的图片模型（`api_format: images`，`capabilities: [image_generation]`），并设置 `output: {kind: image, action: generate}` |
| 带音频查询的 `embeddings` 规则 | 无法改写：删除它；目前还没有能读取音频的决策模型 |

### 响应头与网关 {#headers-and-gateways}

响应头不是配置，migrate 不会改写读取它们的地方。请更新网关和客户端：

| 之前 | 之后 |
| --- | --- |
| 基于 `x-selected-model` 的自定义 Envoy 路由 | `x-vsr-route-model` |
| `x-disable-router-memory` | `x-vsr-memory: off` |
| `x-vsr-matched-*`、决策、置信度和协议相关的头 | `x-vsr-trace` 的成员，只发给 `global.router.headers.debug` 允许的调用方 |
| `x-vsr-cache-hit`、`x-vsr-fast-response` | `x-vsr-response-path` |
| `x-vsr-protocol-warnings`、`x-vsr-response-warnings` | `x-vsr-warnings` |
| `x-vsr-selected-modality` | trace 的 `modality` 成员 |
| `x-session-id`、`x-claude-code-session-id` | `x-vsr-session-id`；`global.router.headers.session_id_headers` 让其他名称继续被接受 |

[VSR 路由头部](../troubleshooting/vsr-headers)页面列出了公开集合和每个 trace 成员。

## 4. 校验并启动 {#4-validate-and-start}

```bash
vllm-sr config validate --config config.migrated.yaml
vllm-sr serve --config config.migrated.yaml
```

## 预期的行为变化 {#behavior-to-expect}

- **原来属于其他模型的阈值已重新拟合。** 没有 `predicate` 的规则使用回答模型的校准阈值；没有校准阈值的模型使用 0.5，并给出 `uncalibrated_preset` 警告。
- **需要 span 的规则必须解析到支持 span 的部署。** `pii`、`hallucination`、`span` 问题和 masking 在不支持 span 的部署（例如 d3 模型）上会以 `span_required` 加载失败。
- **模型没有给出答案的 `jailbreak` 或 `pii` 规则不匹配**，除非设置了 `on_unknown: match`。
- **RAG 相关性和工具选择由决策模型评判**，语义缓存命中由它的 `same_request` 问题确认。
- **嵌入模型只在召回功能需要时才启动**，Router 的嵌入 API 已移除。
- **每个评审都有 `max_candidates` 上限**；更多的候选会被截断，绝不报错。
- **System One 批量调用只使用 `states` 形式**，Router 需要契约 4.0.0 的模型运行时；见[批量请求](../system-one/api#the-batch-request)。
- **图片决策声明 `output` 契约**，它指定的每个模型都声明所需的能力。`POST /v1/images/generations` 只会到达这样的决策；其他请求会得到 `image_route_required`。
- **响应头是一组公开的头**，调试细节放在一个 `x-vsr-trace` 头中；[发布说明](../release-notes/decision-model-first-class#headers)把每个旧响应头映射到它的替代者。
