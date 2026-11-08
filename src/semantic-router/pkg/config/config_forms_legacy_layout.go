package config

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// previousReleaseLayout is what the layouts before config v0.3 become: the
// configuration the previous release's migrate writes, which this release's
// migrate then rewrites.
const previousReleaseLayout = "config v0.3, which `vllm-sr config migrate` of vllm-sr 0.4 writes; then run this one"

// previousReleaseForms are the layouts before config v0.3 that the previous
// release's Router refused and its migrate rewrote: flat top-level signals,
// decisions, endpoints and global blocks, the provider catalog's earlier
// fields, and evaluations kept on model cards. This release refuses them and
// names the release that rewrites them; they run first, so migrate refuses a
// document written in them before it rewrites anything else.
var previousReleaseForms = []ConfigForm{
	{
		Code:     CodeRemovedSection,
		Paths:    []string{"*"},
		Match:    func(_ *yaml.Node, path string) bool { return slices.Contains(preV03TopLevelKeys, path) },
		Describe: describePreviousRelease("top-level %s belongs to the layout before config v0.3"),
	},
	{
		Code:     CodeRemovedField,
		Paths:    []string{"global.runtime", "global.models", "global.modules"},
		Describe: describePreviousRelease("%s belongs to the global layout before config v0.3"),
	},
	{
		Code: CodeRemovedField,
		Paths: []string{
			"providers.model_targets", "providers.backends", "providers.auth_profiles",
			"providers.default_model", "providers.reasoning_families", "providers.default_reasoning_effort",
			"providers.defaults.default_model", "providers.defaults.reasoning_families", "providers.defaults.default_reasoning_effort",
			"providers.models[].access", "providers.models[].endpoints", "providers.models[].access_key",
			"providers.models[].quality_score", "providers.models[].reasoning_family", "providers.models[].evaluations",
			"providers.models[].backend_refs[].type",
			"routing.models", "routing.modelCards[].evaluations", "routing.modelCards[].quality_score",
		},
		Describe: describePreviousRelease("%s belongs to the provider catalog before config v0.3"),
	},
}

// preV03TopLevelKeys are the top-level keys of the layouts before config
// v0.3: flat routing collections, endpoints and model settings, and the
// global blocks written at the top level.
var preV03TopLevelKeys = []string{
	"signals", "decisions", "keyword_rules", "categories", "fact_check_rules", "user_feedback_rules", "language_rules",
	"context_rules", "complexity_rules", "modality_rules", "role_bindings", "jailbreak", "hallucination", "pii",
	"conversation", "metadata", "input_modality",
	"model_config", "vllm_endpoints", "provider_profiles", "default_model", "reasoning_families", "default_reasoning_effort",
	"evaluation_catalog", "auto_model_names",
	"strategy", "auto_model_name", "list_backend_models", "include_config_models_in_list", "clear_route_cache", "model_selection",
	"streamed_body_mode", "max_streamed_body_bytes", "streamed_body_timeout_sec",
	"response_api", "router_replay", "api", "observability", "authz", "ratelimit",
	"semantic_cache", "response_cache", "memory", "vector_store", "tools", "looper",
	"system_models", "external_models", "embedding_models", "prompt_compression", "modality_detector", "bert_model",
	"prompt_guard", "feedback_detector", "classifier", "hallucination_mitigation",
}

func describePreviousRelease(what string) func(FormMatch) (string, string) {
	return func(at FormMatch) (string, string) {
		return fmt.Sprintf(what, at.Path), previousReleaseLayout
	}
}

// legacyRouterForms are router settings the previous release read that
// entrypoints and in-process Loopers replaced: the auto model names, the
// backend model listing switch's earlier name and the Looper's loopback
// endpoint.
var legacyRouterForms = []ConfigForm{
	{
		Code:    CodeRemovedField,
		Paths:   []string{"global.router.auto_model_names", "global.router.auto_model_name"},
		What:    "the auto model names are removed",
		Becomes: "the model_names of the default recipe's entrypoint",
		Migrate: migrateAutoModelNames,
	},
	{
		Code:    CodeRemovedField,
		Paths:   []string{"global.router.include_config_models_in_list"},
		What:    "include_config_models_in_list is removed",
		Becomes: "global.router.list_backend_models",
		Migrate: func(m *Migration, at FormMatch) error {
			if mappingValue(at.Parent, "list_backend_models") != nil {
				m.Delete(at.Parent, "include_config_models_in_list")
				return nil
			}
			return m.Rename(at.Parent, "include_config_models_in_list", "list_backend_models")
		},
	},
	{
		Code:    CodeRemovedField,
		Paths:   []string{"global.integrations.looper.endpoint"},
		What:    "the Looper endpoint is removed",
		Becomes: "nothing; the Router makes a Looper's model calls in process, through each model's providers.models[].backend_refs",
		Migrate: func(m *Migration, at FormMatch) error {
			m.Delete(at.Parent, "endpoint")
			deleteEmpty(m.Root(), "global", "integrations", "looper")
			return nil
		},
	},
	{
		Code: CodeRemovedField,
		Paths: []string{
			"global.integrations.looper.remom.model_names",
			"global.integrations.looper.fusion.model_names",
			"global.integrations.looper.flow.model_names",
		},
		Describe: func(at FormMatch) (string, string) {
			algorithm := strings.TrimSuffix(strings.TrimPrefix(at.Path, "global.integrations.looper."), ".model_names")
			return "the model names of a Looper algorithm are removed",
				"an entrypoint whose model_names name a recipe with a decision that runs the " + algorithm + " algorithm"
		},
	},
}

// migrateAutoModelNames writes the auto model names as the model_names of
// the default recipe's chat entrypoint: the one migrate added for the
// built-in name, or a new one; an entrypoint the document wrote for the
// default recipe keeps its own names.
func migrateAutoModelNames(m *Migration, at FormMatch) error {
	var names []string
	for _, key := range []string{"auto_model_names", "auto_model_name"} {
		value := resolveAlias(mappingValue(at.Parent, key))
		switch {
		case value == nil:
		case value.Kind == yaml.SequenceNode:
			names = append(names, stringItems(value)...)
		case strings.TrimSpace(value.Value) != "":
			names = append(names, value.Value)
		}
		m.Delete(at.Parent, key)
	}
	deleteEmpty(m.Root(), "global", "router")
	names = slices.Compact(slices.DeleteFunc(names, func(name string) bool { return strings.TrimSpace(name) == "" }))
	if len(names) == 0 {
		return nil
	}
	for _, entrypoint := range sequenceAt(m.Root(), "entrypoints") {
		entrypoint = resolveAlias(entrypoint)
		if scalarAt(entrypoint, "recipe") != string(DefaultRecipeName) {
			continue
		}
		if written := stringItems(mappingValue(entrypoint, "model_names")); !slices.Equal(written, []string{DefaultEntrypointModel}) {
			m.Note("entrypoints", fmt.Sprintf("the auto model names %s are dropped: the document's entrypoint of the default recipe names its own models", strings.Join(names, ", ")))
			return nil
		}
		m.Set(entrypoint, "model_names", stringSequence(names...))
		return nil
	}
	entrypoint := mappingNode("model_names", stringSequence(names...), "recipe", string(DefaultRecipeName))
	marked := slices.ContainsFunc(sequenceAt(m.Root(), "entrypoints"), func(e *yaml.Node) bool { return scalarAt(resolveAlias(e), "default") == "true" })
	if len(sequenceAt(m.Root(), "recipes")) > 1 && !marked {
		setNode(entrypoint, "default", boolNode(true))
	}
	sequence := ensureSequence(m.Root(), "entrypoints")
	sequence.Content = append([]*yaml.Node{entrypoint}, sequence.Content...)
	return nil
}
