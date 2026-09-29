package acp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

func TestAgent_ModelSwitcherContracts(t *testing.T) {
	var _ core.ModelSwitcher = (*Agent)(nil)
}

func TestAgent_AvailableModels_DynamicTakesPriority(t *testing.T) {
	a := &Agent{}

	// When no dynamic models reported, falls back to config / modern defaults
	initial := a.AvailableModels(context.Background())
	if len(initial) == 0 {
		t.Fatal("want fallback models, got empty")
	}

	// Session reports dynamic configOptions from server
	a.reportModels("kimi-code/k3", []core.ModelOption{
		{Name: "kimi-code/k3", Alias: "K3", Desc: "Dynamic K3"},
		{Name: "genai/gemini-3.8-flash", Alias: "Gemini", Desc: "Dynamic Gemini"},
	})

	dynamic := a.AvailableModels(context.Background())
	if len(dynamic) != 2 || dynamic[0].Name != "kimi-code/k3" {
		t.Fatalf("AvailableModels = %+v, want dynamic models", dynamic)
	}

	if got := a.GetModel(); got != "kimi-code/k3" {
		t.Fatalf("GetModel = %q, want kimi-code/k3", got)
	}

	a.SetModel("genai/gemini-3.8-flash")
	if got := a.GetModel(); got != "genai/gemini-3.8-flash" {
		t.Fatalf("GetModel after SetModel = %q, want genai/gemini-3.8-flash", got)
	}
}

// parseKimiConfigFile is exercised hermetically via a temp config — the real
// lookup paths depend on the machine's ~/.kimi-code, which CI cannot assume.
func TestParseKimiConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")
	content := `
default_model = "kimi-code/k3"

[models."kimi-code/k3"]
display_name = "K3"
model = "k3"

[models."genai/gemini-3.8-flash"]
model = "gemini-3.8-flash"
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	models := parseKimiConfigFile(cfg)
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2: %+v", len(models), models)
	}
	// default_model sorts first
	if models[0].Name != "kimi-code/k3" || models[0].Alias != "K3" {
		t.Errorf("models[0] = %+v, want kimi-code/k3 (K3) first", models[0])
	}
	// missing display_name falls back to the id as alias
	if models[1].Name != "genai/gemini-3.8-flash" || models[1].Alias != "genai/gemini-3.8-flash" {
		t.Errorf("models[1] = %+v, want id-as-alias fallback", models[1])
	}

	if got := parseKimiConfigFile(filepath.Join(dir, "missing.toml")); got != nil {
		t.Errorf("missing file: got %+v, want nil", got)
	}
}
