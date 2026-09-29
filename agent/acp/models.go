package acp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/chenhg5/cc-connect/core"
)

// Model discovery for ACP backends that expose no model-listing RPC —
// concretely Kimi Code CLI, whose available models live in its local
// config.toml. Self-contained by design: agent packages must not import
// each other, so the equivalent logic in agent/kimi cannot be reused here.

// defaultModernKimiModels provides modern defaults when no local config exists,
// replacing the legacy K2-0711 models.
var defaultModernKimiModels = []core.ModelOption{
	{Name: "kimi-code/k3", Alias: "K3", Desc: "Kimi K3 (most capable)"},
	{Name: "kimi-code/kimi-for-coding", Alias: "K2.8", Desc: "K2.8 Preview"},
	{Name: "kimi-code/kimi-for-coding-highspeed", Alias: "K2.7", Desc: "K2.7 Code Highspeed"},
	{Name: "kimi-code/k3-256k", Alias: "K3-256k", Desc: "K3 256k context"},
	{Name: "genai/gemini-3.8-flash", Alias: "Gemini 3.8 Flash", Desc: "Gemini 3.8 Flash (GenAI Bridge)"},
	{Name: "dmp/k3", Alias: "dmp/k3", Desc: "DMP K3 (endor)"},
}

// loadKimiConfigModels parses model definitions from ~/.kimi-code/config.toml
// (falling back to ~/.kimi/config.toml).
func loadKimiConfigModels() []core.ModelOption {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultModernKimiModels
	}

	paths := []string{
		filepath.Join(home, ".kimi-code", "config.toml"),
		filepath.Join(home, ".kimi", "config.toml"),
	}

	for _, p := range paths {
		if models := parseKimiConfigFile(p); len(models) > 0 {
			return models
		}
	}

	return defaultModernKimiModels
}

type kimiConfigModelEntry struct {
	DisplayName string `toml:"display_name"`
	Model       string `toml:"model"`
	Provider    string `toml:"provider"`
}

type kimiConfigDocument struct {
	DefaultModel string                          `toml:"default_model"`
	Models       map[string]kimiConfigModelEntry `toml:"models"`
}

func parseKimiConfigFile(path string) []core.ModelOption {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var doc kimiConfigDocument
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil
	}

	if len(doc.Models) == 0 {
		return nil
	}

	options := make([]core.ModelOption, 0, len(doc.Models))
	for id, entry := range doc.Models {
		alias := entry.DisplayName
		if alias == "" {
			alias = id
		}
		desc := entry.DisplayName
		if desc == "" && entry.Model != "" {
			desc = entry.Model
		}

		options = append(options, core.ModelOption{
			Name:  id,
			Alias: alias,
			Desc:  desc,
		})
	}

	// Sort models so default_model appears first, followed by others sorted by Name
	defaultModel := strings.TrimSpace(doc.DefaultModel)
	sort.Slice(options, func(i, j int) bool {
		if options[i].Name == defaultModel {
			return true
		}
		if options[j].Name == defaultModel {
			return false
		}
		return options[i].Name < options[j].Name
	})

	return options
}
