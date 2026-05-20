package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// TeamProfile represents the local configuration for the team
type TeamProfile struct {
	TeamID      string   `json:"team_id"`
	TeamName    string   `json:"team_name"`
	Description string   `json:"description"`
	TechStack   []string `json:"tech_stack"`
}

// GetConfigPath returns the path to ~/.devask/config.json
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".devask", "config.json"), nil
}

// InitProfile creates a new team profile
func InitProfile(name, desc string, stack []string) error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	profile := TeamProfile{
		TeamID:      uuid.New().String(),
		TeamName:    name,
		Description: desc,
		TechStack:   stack,
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	fmt.Printf("Team profile created successfully at %s\n", path)
	fmt.Printf("Team ID: %s\n", profile.TeamID)
	return nil
}

// LoadProfile loads the existing profile
func LoadProfile() (*TeamProfile, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config: %v", err)
	}

	var profile TeamProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, err
	}

	return &profile, nil
}
