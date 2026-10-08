package compact

import (
	"agent/config"
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/anthropics/anthropic-sdk-go"
)

// 大结果落盘
func ToolResultBudget(messages []anthropic.MessageParam, maxBytes int) []anthropic.MessageParam {
	if len(messages) == 0 {
		return messages
	}
	last := &messages[len(messages)-1]
	indeices := make([]int, 0)
	total := 0
	for i, block := range last.Content {
		if block.OfToolResult != nil {
			indeices = append(indeices, i)
			total += toolResultSize(block)
		}
	}

	if total <= maxBytes {
		return messages
	}
	slices.SortFunc(indeices, func(a, b int) int {
		return cmp.Compare(
			toolResultSize(last.Content[b]),
			toolResultSize(last.Content[a]),
		)
	})

	for _, index := range indeices {
		if total <= maxBytes {
			break
		}
		block := last.Content[index]
		toolUseID := block.OfToolResult.ToolUseID
		output := ""
		for _, content := range block.OfToolResult.Content {
			if content.OfText != nil {
				output += content.OfText.Text
			}
		}

		oldSize := toolResultSize(block)
		persisted := persistLargeOutput(toolUseID, output)

		isError := false
		if block.OfToolResult.IsError.Valid() {
			isError = block.OfToolResult.IsError.Value
		}

		last.Content[index] = anthropic.NewToolResultBlock(
			toolUseID,
			persisted,
			isError,
		)

		newSize := toolResultSize(last.Content[index])
		total = total - oldSize + newSize
	}
	return messages

}

func toolResultSize(block anthropic.ContentBlockParamUnion) int {
	if block.OfToolResult == nil {
		return 0
	}
	total := 0
	for _, content := range block.OfToolResult.Content {
		if content.OfText != nil {
			total += len(content.OfText.Text)
		}
	}
	return total
}

func persistLargeOutput(toolUseID string, output string) string {
	if len(output) <= config.PERSIST_THRESHOLD {
		return output
	}

	err := os.MkdirAll(config.TOOL_RESULTS_DIR, 0755)
	if err != nil {
		return output
	}

	path := filepath.Join(
		config.TOOL_RESULTS_DIR,
		toolUseID+".txt",
	)

	_, err = os.Stat(path)

	if err != nil {
		if os.IsNotExist(err) {
			err := os.WriteFile(path, []byte(output), 0644)
			if err != nil {
				return output
			}
		}
	}

	preview := output
	if len(preview) > 2000 {
		preview = preview[:2000]
	}

	return fmt.Sprintf(
		"<persisted-output>\nFull output: %s\nPreview:\n%s\n</persisted-output>",
		path,
		preview,
	)
}
