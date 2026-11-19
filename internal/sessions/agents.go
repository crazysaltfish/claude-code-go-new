package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"claude-code-go/internal/tasks"
)

const agentRecordVersion = 1

// AgentRecord is stored separately from the parent model context and history.
type AgentRecord struct {
	Version int                        `json:"version"`
	Task    *tasks.LocalAgentTaskState `json:"task"`
	Entries []AgentEntry               `json:"entries"`
}

type AgentEntry struct {
	ID        int          `json:"id"`
	Kind      string       `json:"kind"`
	Tool      string       `json:"tool,omitempty"`
	CallID    string       `json:"callId,omitempty"`
	Text      string       `json:"text"`
	Time      int64        `json:"time"`
	Truncated bool         `json:"truncated,omitempty"`
	Output    *AgentOutput `json:"output,omitempty"`
}

type AgentOutput struct {
	Text      string `json:"text"`
	Time      int64  `json:"time"`
	Failed    bool   `json:"failed,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

func (s *Store) agentDir(sessionID string) (string, error) {
	if !validID.MatchString(sessionID) {
		return "", fmt.Errorf("invalid parent session ID %q", sessionID)
	}
	return filepath.Join(s.dir, "agents", sessionID), nil
}

func (s *Store) SaveAgent(record AgentRecord) error {
	task := record.Task
	if task == nil || !validID.MatchString(task.ID) {
		return fmt.Errorf("invalid agent task")
	}
	dir, err := s.agentDir(task.SessionID)
	if err != nil {
		return err
	}
	record.Version = agentRecordVersion
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return writePrivateSnapshot(dir, filepath.Join(dir, task.ID+".json"), data)
}

func (s *Store) LoadAgent(sessionID, taskID string) (AgentRecord, error) {
	var record AgentRecord
	if !validID.MatchString(taskID) {
		return record, fmt.Errorf("invalid agent ID %q", taskID)
	}
	dir, err := s.agentDir(sessionID)
	if err != nil {
		return record, err
	}
	path := filepath.Join(dir, taskID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() {
		return record, fmt.Errorf("agent record must be a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if record.Version != agentRecordVersion || record.Task == nil ||
		record.Task.ID != taskID || record.Task.SessionID != sessionID ||
		record.Task.Type != tasks.TaskTypeLocalAgent {
		return record, fmt.Errorf("invalid agent record %q", taskID)
	}
	// An archived running task belonged to a previous process.
	if record.Task.Status == tasks.TaskStatusRunning || record.Task.Status == tasks.TaskStatusPending {
		record.Task.Status = tasks.TaskStatusInterrupted
	}
	return record, nil
}

func (s *Store) ListAgents(sessionID string) (map[string]tasks.TaskState, error) {
	dir, err := s.agentDir(sessionID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]tasks.TaskState)
	files, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		id, ok := strings.CutSuffix(file.Name(), ".json")
		if !ok {
			continue
		}
		record, err := s.LoadAgent(sessionID, id)
		if err != nil {
			return nil, fmt.Errorf("load agent record %q: %w", id, err)
		}
		result[id] = record.Task
	}
	return result, nil
}
