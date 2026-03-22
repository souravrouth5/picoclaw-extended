package mcp

// Workflow is an ordered list of tool names that form a logical RE pipeline.
type Workflow struct {
	Name        string
	Description string
	Steps       []WorkflowStep
}

// WorkflowStep is a single tool call with default parameters.
type WorkflowStep struct {
	Tool        string
	Params      map[string]interface{}
	RetryPolicy int // max retries on recoverable failure
}

// Workflows maps intent keywords to predefined pipelines.
// The planner selects the best matching workflow from user input.
var Workflows = map[string]Workflow{
	"apk_triage": {
		Name:        "apk_triage",
		Description: "Quick recon: manifest, contents, decompile, string search",
		Steps: []WorkflowStep{
			{Tool: "extract_manifest", Params: map[string]interface{}{}},
			{Tool: "list_apk_contents", Params: map[string]interface{}{}},
			{Tool: "decompile_apk", Params: map[string]interface{}{"tool": "both"}},
			{Tool: "search_strings", Params: map[string]interface{}{"pattern": "(api_key|secret|password|token|http)", "context_lines": 2}},
			{Tool: "detect_protections", Params: map[string]interface{}{}},
		},
	},
	"protection_bypass": {
		Name:        "protection_bypass",
		Description: "Detect and patch SSL pinning, root detection, emulator detection",
		Steps: []WorkflowStep{
			{Tool: "detect_protections", Params: map[string]interface{}{}},
			{Tool: "patch_ssl_pinning", Params: map[string]interface{}{"strategy": "smali"}, RetryPolicy: 1},
			{Tool: "patch_root_detection", Params: map[string]interface{}{"strategy": "smali"}, RetryPolicy: 1},
			{Tool: "patch_emulator_detection", Params: map[string]interface{}{"strategy": "smali"}, RetryPolicy: 1},
			{Tool: "rebuild_and_sign", Params: map[string]interface{}{}},
		},
	},
	"dynamic_analysis": {
		Name:        "dynamic_analysis",
		Description: "Frida-based runtime analysis with combined hooks",
		Steps: []WorkflowStep{
			{Tool: "get_capabilities", Params: map[string]interface{}{}},
			{Tool: "frida_capture", Params: map[string]interface{}{"duration": 30}, RetryPolicy: 1},
			{Tool: "logcat", Params: map[string]interface{}{"duration_seconds": 30}},
		},
	},
	"full_analysis": {
		Name:        "full_analysis",
		Description: "Complete pipeline: triage → bypass → dynamic → interpret",
		Steps: []WorkflowStep{
			{Tool: "extract_manifest", Params: map[string]interface{}{}},
			{Tool: "list_apk_contents", Params: map[string]interface{}{}},
			{Tool: "decompile_apk", Params: map[string]interface{}{"tool": "both"}},
			{Tool: "search_strings", Params: map[string]interface{}{"pattern": "(api_key|secret|password|token|http)", "context_lines": 2}},
			{Tool: "detect_protections", Params: map[string]interface{}{}},
			{Tool: "patch_ssl_pinning", Params: map[string]interface{}{"strategy": "smali"}, RetryPolicy: 1},
			{Tool: "patch_root_detection", Params: map[string]interface{}{"strategy": "smali"}, RetryPolicy: 1},
			{Tool: "rebuild_and_sign", Params: map[string]interface{}{}},
			{Tool: "get_capabilities", Params: map[string]interface{}{}},
			{Tool: "frida_capture", Params: map[string]interface{}{"duration": 30}, RetryPolicy: 1},
			{Tool: "logcat", Params: map[string]interface{}{"duration_seconds": 30}},
			{Tool: "interpret_findings", Params: map[string]interface{}{}},
		},
	},
}

// intentKeywords maps user intent phrases to workflow names.
var intentKeywords = map[string][]string{
	"apk_triage":        {"triage", "quick", "recon", "scan", "what is", "analyze", "analyse"},
	"protection_bypass": {"bypass", "patch", "ssl", "pinning", "root", "detection", "unpin"},
	"dynamic_analysis":  {"dynamic", "frida", "runtime", "hook", "intercept", "traffic"},
	"full_analysis":     {"full", "complete", "everything", "all"},
}

// SelectWorkflow picks the best workflow for a given free-form intent string.
// Falls back to "apk_triage" if nothing matches.
func SelectWorkflow(intent string) Workflow {
	lower := toLower(intent)
	bestMatch := ""
	bestScore := 0
	for wf, keywords := range intentKeywords {
		score := 0
		for _, kw := range keywords {
			if contains(lower, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestMatch = wf
		}
	}
	if bestMatch == "" {
		bestMatch = "apk_triage"
	}
	return Workflows[bestMatch]
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
