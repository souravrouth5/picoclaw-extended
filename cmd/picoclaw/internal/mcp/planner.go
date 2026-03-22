package mcp

// Plan is a structured execution plan produced by the planner.
type Plan struct {
	Goal     string
	Workflow string
	Target   string // package name or apk path
	Mode     string // full | static | dynamic | quick
	Focus    string // crypto | network | auth | root-detection | ""
	Steps    []PlanStep
}

// PlanStep is a resolved tool call with injected parameters.
type PlanStep struct {
	Tool        string
	Params      map[string]interface{}
	RetryPolicy int
}

// BuildPlan creates an execution plan from user intent and target.
func BuildPlan(intent, target, mode, focus string) Plan {
	wf := selectWorkflowForMode(intent, mode)

	steps := make([]PlanStep, len(wf.Steps))
	for i, s := range wf.Steps {
		params := copyParams(s.Params)
		// Inject target into tools that need it.
		injectTarget(s.Tool, params, target)
		// Inject focus where relevant.
		if focus != "" {
			injectFocus(s.Tool, params, focus)
		}
		steps[i] = PlanStep{
			Tool:        s.Tool,
			Params:      params,
			RetryPolicy: s.RetryPolicy,
		}
	}

	return Plan{
		Goal:     intent,
		Workflow: wf.Name,
		Target:   target,
		Mode:     mode,
		Focus:    focus,
		Steps:    steps,
	}
}

func selectWorkflowForMode(intent, mode string) Workflow {
	switch mode {
	case "full":
		return Workflows["full_analysis"]
	case "static":
		return Workflows["apk_triage"]
	case "dynamic":
		return Workflows["dynamic_analysis"]
	case "quick":
		// quick = triage only, no patching
		return Workflows["apk_triage"]
	default:
		return SelectWorkflow(intent)
	}
}

// injectTarget sets the primary target parameter for tools that need it.
func injectTarget(tool string, params map[string]interface{}, target string) {
	switch tool {
	case "extract_manifest", "list_apk_contents", "detect_protections":
		params["apk_path"] = target
	case "decompile_apk":
		params["apk_path"] = target
	case "search_strings":
		params["target"] = target
	case "patch_ssl_pinning", "patch_root_detection", "patch_emulator_detection":
		// source_dir is set by executor after decompile step completes
	case "rebuild_and_sign":
		// source_dir set by executor
	case "frida_capture", "frida_spawn", "frida_attach":
		params["package_name"] = target
	case "logcat":
		params["package_name"] = target
	case "interpret_findings":
		params["target"] = target
	}
}

func injectFocus(tool string, params map[string]interface{}, focus string) {
	if tool == "search_strings" {
		focusPatterns := map[string]string{
			"crypto":          "(AES|DES|RSA|cipher|encrypt|decrypt|key|iv|secret)",
			"network":         "(http|https|url|endpoint|api|host|socket)",
			"auth":            "(auth|token|login|password|credential|session|jwt|oauth)",
			"root-detection":  "(root|su|superuser|busybox|magisk|xposed)",
		}
		if p, ok := focusPatterns[focus]; ok {
			params["pattern"] = p
		}
	}
	if tool == "interpret_findings" {
		params["focus"] = focus
	}
}

func copyParams(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
