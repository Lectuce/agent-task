package tool

import (
	"fmt"

	"github.com/Knetic/govaluate"
)

func runCalculator(input map[string]any) (string, error) {
	expression, err := requireString(input, "expression", "expression is required")
	if err != nil {
		return "", err
	}

	expr, err := govaluate.NewEvaluableExpression(expression)
	if err != nil {
		return "", fmt.Errorf("invalid expression: %w", err)
	}

	result, err := expr.Evaluate(nil)
	if err != nil {
		return "", fmt.Errorf("calculate expression: %w", err)
	}

	return fmt.Sprintf("%v", result), nil
}
