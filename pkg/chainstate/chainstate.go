package chainstate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type State struct {
	Projects map[string]*ProjectState `json:"projects"`
}

type ProjectState struct {
	ActiveChain string         `json:"active_chain,omitempty"`
	Chains      map[string]int `json:"chains,omitempty"`
}

func DefaultPath() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "cheatmd", "chains.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "cheatmd", "chains.json"), nil
}

func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Projects: make(map[string]*ProjectState)}, nil
		}
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err == nil {
		if state.Projects == nil {
			state.Projects = make(map[string]*ProjectState)
		}
		return &state, nil
	}

	// Fallback to old format migration
	var old map[string]int
	if err := json.Unmarshal(data, &old); err == nil {
		return migrateOldState(old), nil
	}

	// Unmarshal failed completely, return empty state
	return &State{Projects: make(map[string]*ProjectState)}, nil
}

func migrateOldState(old map[string]int) *State {
	s := &State{Projects: make(map[string]*ProjectState)}
	for key, value := range old {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) == 2 {
			// root|name
			root := parts[0]
			name := parts[1]
			p := getOrCreateProject(s, root)
			p.Chains[name] = value
		} else if len(parts) == 3 && parts[1] == "@active" {
			// root|@active|name
			root := parts[0]
			name := parts[2]
			if value > 0 {
				p := getOrCreateProject(s, root)
				p.ActiveChain = name
			}
		}
	}
	return s
}

func getOrCreateProject(state *State, root string) *ProjectState {
	cleanRoot := filepath.Clean(root)
	p, ok := state.Projects[cleanRoot]
	if !ok {
		p = &ProjectState{Chains: make(map[string]int)}
		state.Projects[cleanRoot] = p
	}
	if p.Chains == nil {
		p.Chains = make(map[string]int)
	}
	return p
}

func Save(path string, state *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func ActiveName(root string, state *State) string {
	if state == nil || state.Projects == nil {
		return ""
	}
	if p, ok := state.Projects[filepath.Clean(root)]; ok {
		return p.ActiveChain
	}
	return ""
}

func SetActive(root, name string, state *State) {
	if state == nil {
		return
	}
	p := getOrCreateProject(state, root)
	p.ActiveChain = name
}

func Clear(root, name string, state *State) {
	if state == nil || state.Projects == nil {
		return
	}
	cleanRoot := filepath.Clean(root)
	p, ok := state.Projects[cleanRoot]
	if !ok {
		return
	}
	if name == "" {
		// Clear all chains and active status for this project
		delete(state.Projects, cleanRoot)
		return
	}
	delete(p.Chains, name)
	if p.ActiveChain == name {
		p.ActiveChain = ""
	}
}

func GetStep(root, name string, state *State) int {
	if state == nil || state.Projects == nil {
		return 0
	}
	if p, ok := state.Projects[filepath.Clean(root)]; ok {
		return p.Chains[name]
	}
	return 0
}

func SetStep(root, name string, step int, state *State) {
	if state == nil {
		return
	}
	p := getOrCreateProject(state, root)
	p.Chains[name] = step
}
