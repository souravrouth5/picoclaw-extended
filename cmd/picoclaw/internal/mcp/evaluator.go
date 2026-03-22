package mcp

// StepResult holds the parsed outcome of a single tool call.
type StepResult struct {
	Status            string                 // "success" | "error"
	ErrorType         string                 // e.g. "frida_detection", "apktool_failure"
	Recoverable       bool
	SuggestedNext     []string               // from tool's suggested_next_steps
	Data              map[string]interface{} // full parsed response
}

// Decision is what the evaluator tells the executor to do next.
type Decision int

const (
	DecisionContinue Decision = iota // proceed to next step normally
	DecisionRetry                    // retry the same step
	DecisionAdapt                    // replace current step with an alternative
	DecisionSkip                     // skip this step, continue
	DecisionAbort                    // unrecoverable, stop the plan
)

// EvalResult is returned by Evaluate.
type EvalResult struct {
	Decision    Decision
	AdaptedStep *PlanStep // set when Decision == DecisionAdapt
	Reason      string
}

// Evaluate inspects a step result and returns what to do next.
func Evaluate(step PlanStep, result StepResult, attempt int) EvalResult {
	if result.Status == "success" {
		return EvalResult{Decision: DecisionContinue}
	}

	if !result.Recoverable {
		return EvalResult{
			Decision: DecisionAbort,
			Reason:   "unrecoverable error: " + result.ErrorType,
		}
	}

	// Retry budget not exhausted.
	if attempt < step.RetryPolicy {
		return EvalResult{
			Decision: DecisionRetry,
			Reason:   "recoverable error, retrying: " + result.ErrorType,
		}
	}

	// Apply adaptive strategies based on error type.
	adapted := adaptStep(step, result)
	if adapted != nil {
		return EvalResult{
			Decision:    DecisionAdapt,
			AdaptedStep: adapted,
			Reason:      "adapting strategy for: " + result.ErrorType,
		}
	}

	// No adaptation available — skip this step and continue.
	return EvalResult{
		Decision: DecisionSkip,
		Reason:   "no adaptation available, skipping: " + result.ErrorType,
	}
}

// adaptStep returns an alternative PlanStep for known failure patterns.
// Returns nil if no adaptation is available.
func adaptStep(step PlanStep, result StepResult) *PlanStep {
	switch result.ErrorType {
	case "frida_detection":
		// frida_attach failed due to detection → try frida_spawn instead
		if step.Tool == "frida_attach" {
			adapted := copyPlanStep(step)
			adapted.Tool = "frida_spawn"
			adapted.Params["delay_hooks"] = true
			return &adapted
		}
	case "frida_spawn_failure":
		// spawn failed → fall back to static patching
		if step.Tool == "frida_spawn" || step.Tool == "frida_attach" {
			return &PlanStep{
				Tool:   "patch_ssl_pinning",
				Params: map[string]interface{}{"strategy": "smali"},
			}
		}
	case "apktool_failure":
		// apktool failed → try jadx-only decompile
		if step.Tool == "decompile_apk" {
			adapted := copyPlanStep(step)
			adapted.Params["tool"] = "jadx"
			return &adapted
		}
	case "adb_not_found", "device_not_connected":
		// device tools unavailable → skip silently
		return nil
	}
	return nil
}

func copyPlanStep(s PlanStep) PlanStep {
	params := make(map[string]interface{}, len(s.Params))
	for k, v := range s.Params {
		params[k] = v
	}
	return PlanStep{Tool: s.Tool, Params: params, RetryPolicy: s.RetryPolicy}
}
