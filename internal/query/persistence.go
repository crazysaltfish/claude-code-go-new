package query

import "claude-code-go/internal/types"

// Called with the engine lock held. History survives model-context compaction.
func (e *QueryEngine) appendMessages(messages ...types.Message) {
	e.mutableMessages = append(e.mutableMessages, messages...)
	e.history = append(e.history, messages...)
	e.persistenceDirty = true
}

func (e *QueryEngine) checkpoint(output chan<- interface{}) bool {
	if e.config.PersistSession == nil || !e.persistenceDirty {
		return true
	}
	e.persistenceDirty = false
	if err := e.config.PersistSession(e.mutableMessages, e.history); err != nil {
		output <- e.createErrorResultMessage("session_save_error", "Failed to save session: "+err.Error(), 0)
		return false
	}
	return true
}
