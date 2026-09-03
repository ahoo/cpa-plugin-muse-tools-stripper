package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsZenFree(t *testing.T) {
	yes := []string{"muse-free", "muse", "MUSE-FREE", "muse-spark-1.3-contributor-free", "provider/muse-free", "muse-spark-1.4-foo-free"}
	for _, m := range yes {
		if !isZenFree(m) {
			t.Errorf("isZenFree(%q) = false, want true", m)
		}
	}
	no := []string{"", "gpt-5", "deepseek-flash", "muse-spark-1.3-contributor", "muse-spark-contributor", "claude-opus-4-8"}
	for _, m := range no {
		if isZenFree(m) {
			t.Errorf("isZenFree(%q) = true, want false", m)
		}
	}
}

func TestStripUnsupportedInput(t *testing.T) {
	in := []byte(`{"model":"muse-free","input":[{"type":"additional_tools","tools":[]},{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},{"type":"reasoning","summary":[]}]}`)
	out, changed := stripUnsupportedInput(in)
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	var decoded struct {
		Input []struct {
			Type string `json:"type"`
		} `json:"input"`
	}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Input) != 2 || decoded.Input[0].Type != "message" || decoded.Input[1].Type != "reasoning" {
		t.Fatalf("unexpected surviving entries: %s", out)
	}
}

func TestStripPassthrough(t *testing.T) {
	in := []byte(`{"model":"muse-free","input":[{"type":"message","role":"user","content":[]}]}`)
	if _, changed := stripUnsupportedInput(in); changed {
		t.Fatalf("changed = true for clean input")
	}
	if _, changed := stripUnsupportedInput([]byte(`{"model":"x"}`)); changed {
		t.Fatalf("changed = true without input array")
	}
}

func TestInterceptGating(t *testing.T) {
	body := []byte(`{"model":"muse-free","input":[{"type":"additional_tools"}]}`)
	mk := func(model, requested string) []byte {
		raw, _ := json.Marshal(interceptRequest{SourceFormat: "codex", Model: model, RequestedModel: requested, Body: body})
		return raw
	}
	// zen free alias -> stripped
	raw, err := intercept(mk("muse-spark-1.3-contributor-free", "muse-free"))
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	var resp interceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Body) == 0 || strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("expected stripped body, got: %s", resp.Body)
	}
	// unrelated model -> untouched
	raw, err = intercept(mk("gpt-5", "gpt-5"))
	if err != nil {
		t.Fatal(err)
	}
	env = envelope{}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	resp = interceptResponse{}
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Body) != 0 {
		t.Fatalf("expected passthrough, got: %s", resp.Body)
	}
}
