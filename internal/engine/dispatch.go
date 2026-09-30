package engine

import (
	"fmt"

	"github.com/hoshinoht/shiori/internal/ojson"
)

// Prepare dispatches a mutating tool ("create", "update", "patch",
// "reset", "checkpoint", "compact", "compact_preview") on accepted input.
func (e *Engine) Prepare(tool string, data ojson.Value) (*Prepared, error) {
	switch tool {
	case "create":
		return e.PrepareCreate(data)
	case "checkpoint":
		return e.PrepareCheckpoint(data)
	case "compact", "compact_preview":
		return e.PrepareCompact(data)
	case "patch":
		return e.PreparePatch(data)
	case "reset":
		return e.PrepareReset(data)
	case "update":
		return e.PrepareUpdate(data)
	}
	return nil, fmt.Errorf("unsupported mutation tool: %s", tool)
}
