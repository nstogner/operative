package sandbox

import (
	"context"
	"fmt"
)

const ToolNameRunIPythonCell = "run_ipython_cell"

type IPythonCellTool struct{}

func (t *IPythonCellTool) Name() string { return ToolNameRunIPythonCell }

func (t *IPythonCellTool) Description() string {
	return "Run a cell of code in the IPython kernel. Returns the result of running the cell."
}

func (t *IPythonCellTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"code": map[string]any{
				"type":        "string",
				"description": "The code to run.",
			},
		},
		"required": []string{"code"},
	}
}

// Execute is a placeholder. The actual execution should be handled by the Runner
// which has access to the sandbox manager and session ID.
func (t *IPythonCellTool) Execute(ctx context.Context, input map[string]any) (any, error) {
	return nil, fmt.Errorf("this tool should be executed via the sandbox manager")
}
