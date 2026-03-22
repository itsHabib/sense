package sense

import (
	"encoding/json"
	"fmt"
	"strings"
)

const evalSystemPrompt = `You are a strict test evaluator. You will receive output to evaluate and a list of expectations. For each expectation, determine whether the output satisfies it.

Be strict. Only pass an expectation if you are confident the output satisfies it. When in doubt, fail it and explain why.

For each expectation, provide:
- pass: whether the output satisfies it
- confidence: how confident you are (0.0 to 1.0)
- reason: a concise explanation of why it passes or fails
- evidence: specific quotes or references from the output that support your judgment

Set the top-level "pass" to true only if ALL expectations pass.
Set "score" to the fraction of expectations that passed (0.0 to 1.0).`

const compareSystemPrompt = `You are a strict comparator. You will receive two outputs (A and B) and a list of criteria. For each criterion, score both outputs from 0.0 to 1.0 and determine which is better.

Be precise. Tie only when the outputs are genuinely equal on a criterion.

Set "winner" to "A", "B", or "tie" based on which output scores higher overall.`

func buildEvalUserMessage(output string, expectations []string, context string) string {
	var b strings.Builder

	if context != "" {
		fmt.Fprintf(&b, "Context:\n%s\n\n", context)
	}

	fmt.Fprintf(&b, "Output to evaluate:\n\"\"\"\n%s\n\"\"\"\n\n", output)

	b.WriteString("Expectations:\n")
	for i, exp := range expectations {
		fmt.Fprintf(&b, "%d. %s\n", i+1, exp)
	}
	b.WriteString("\nEvaluate each expectation and submit your result.")

	return b.String()
}

func buildCompareUserMessage(outputA, outputB string, criteria []string, context string) string {
	var b strings.Builder

	if context != "" {
		fmt.Fprintf(&b, "Context:\n%s\n\n", context)
	}

	fmt.Fprintf(&b, "Output A:\n\"\"\"\n%s\n\"\"\"\n\n", outputA)
	fmt.Fprintf(&b, "Output B:\n\"\"\"\n%s\n\"\"\"\n\n", outputB)

	b.WriteString("Criteria:\n")
	for i, c := range criteria {
		fmt.Fprintf(&b, "%d. %s\n", i+1, c)
	}
	b.WriteString("\nCompare both outputs on each criterion and submit your result.")

	return b.String()
}

// serializeOutput converts any output type to a string for the prompt.
func serializeOutput(output any) string {
	switch v := output.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case fmt.Stringer:
		return v.String()
	default:
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}
