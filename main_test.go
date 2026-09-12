package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIsZenFree(t *testing.T) {
	yes := []string{"muse-free", "muse", "MUSE-FREE", "muse-spark-1.3-contributor-free", "provider/muse-free", "muse-spark-1.4-foo-free",
		"muse-free(high)", "MUSE-FREE(high)", "provider/muse-free(high)",
		"muse-spark-1.3-contributor-free(high)", "muse-free(8192)", "muse-free(none)"}
	for _, m := range yes {
		if !isZenFree(m) {
			t.Errorf("isZenFree(%q) = false, want true", m)
		}
	}
	no := []string{"", "gpt-5", "deepseek-flash", "muse-spark-1.3-contributor", "muse-spark-contributor", "claude-opus-4-8",
		"muse-spark-1.3-contributor(high)", "gpt-5(high)", "muse-free(high", "muse-free(high)extra"}
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

func decodeIntercept(t *testing.T, raw []byte) interceptResponse {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	var resp interceptResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestInterceptGating(t *testing.T) {
	body := []byte(`{"model":"muse-free","input":[{"type":"additional_tools"}]}`)
	mk := func(source, to, model, requested string) []byte {
		raw, _ := json.Marshal(interceptRequest{SourceFormat: source, ToFormat: to, Model: model, RequestedModel: requested, Body: body})
		return raw
	}
	// zen free alias -> stripped
	raw, err := intercept(mk("codex", "", "muse-spark-1.3-contributor-free", "muse-free"))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decodeIntercept(t, raw); len(resp.Body) == 0 || strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("expected stripped body, got: %s", resp.Body)
	}
	// before-auth shape: ToFormat empty, SourceFormat codex -> stripped
	raw, err = intercept(mk("codex", "", "muse-free(high)", "muse-free(high)"))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decodeIntercept(t, raw); len(resp.Body) == 0 || strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("expected stripped body for before-auth shape, got: %s", resp.Body)
	}
	// router target with thinking suffix -> stripped
	raw, err = intercept(mk("codex", "codex", "muse-spark-1.3-contributor-free(high)", "muse-free(high)"))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decodeIntercept(t, raw); len(resp.Body) == 0 || strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("expected stripped body for suffixed model, got: %s", resp.Body)
	}
	// paid build with thinking suffix -> untouched
	raw, err = intercept(mk("codex", "codex", "muse-spark-1.3-contributor(high)", "muse-spark-1.3-contributor(high)"))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decodeIntercept(t, raw); len(resp.Body) != 0 {
		t.Fatalf("expected passthrough for paid suffixed model, got: %s", resp.Body)
	}
	// unrelated model -> untouched
	raw, err = intercept(mk("codex", "codex", "gpt-5", "gpt-5"))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decodeIntercept(t, raw); len(resp.Body) != 0 {
		t.Fatalf("expected passthrough, got: %s", resp.Body)
	}
	// both formats known non-codex -> untouched
	raw, err = intercept(mk("claude", "claude", "muse-free", "muse-free"))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decodeIntercept(t, raw); len(resp.Body) != 0 {
		t.Fatalf("expected passthrough for non-codex formats, got: %s", resp.Body)
	}
	// outer invalid JSON -> error (host treats as skip, fail-open)
	if _, err := intercept([]byte(`{invalid`)); err == nil {
		t.Fatalf("expected error for invalid outer JSON")
	}
	// invalid inner body -> passthrough without error
	badBody := func() []byte {
		raw, _ := json.Marshal(interceptRequest{SourceFormat: "codex", Model: "muse-free", RequestedModel: "muse-free", Body: []byte(`{invalid`)})
		return raw
	}
	raw, err = intercept(badBody())
	if err != nil {
		t.Fatal(err)
	}
	if resp := decodeIntercept(t, raw); len(resp.Body) != 0 {
		t.Fatalf("expected passthrough for invalid body, got: %s", resp.Body)
	}
}

func TestNormalizeGating(t *testing.T) {
	body := []byte(`{"model":"muse-spark-1.3-contributor-free","input":[{"type":"additional_tools"},{"type":"message"}]}`)
	mk := func(model string, payload []byte) []byte {
		raw, _ := json.Marshal(normalizeRequest{FromFormat: "claude", ToFormat: "codex", Model: model, Body: payload})
		return raw
	}
	decode := func(t *testing.T, raw []byte) normalizeResponse {
		t.Helper()
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatal(err)
		}
		var resp normalizeResponse
		if err := json.Unmarshal(env.Result, &resp); err != nil {
			t.Fatal(err)
		}
		return resp
	}
	// translated payload for zen free model -> stripped
	raw, err := normalize(mk("muse-spark-1.3-contributor-free(high)", body))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); len(resp.Body) == 0 || strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("expected stripped body, got: %s", resp.Body)
	}
	// translated payload for paid model -> untouched
	raw, err = normalize(mk("muse-spark-1.3-contributor(high)", body))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); string(resp.Body) != string(body) {
		t.Fatalf("expected untouched body for paid model, got: %s", resp.Body)
	}
	// translated payload without input[] -> untouched
	clean := []byte(`{"model":"muse-free","messages":[{"role":"user","content":"hi"}]}`)
	raw, err = normalize(mk("muse-free", clean))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); string(resp.Body) != string(clean) {
		t.Fatalf("expected untouched body, got: %s", resp.Body)
	}
	// translated payload toward non-codex format -> untouched even with input[]
	mkFormat := func(toFormat, model string, payload []byte) []byte {
		raw, _ := json.Marshal(normalizeRequest{FromFormat: "claude", ToFormat: toFormat, Model: model, Body: payload})
		return raw
	}
	raw, err = normalize(mkFormat("claude", "muse-free", body))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); string(resp.Body) != string(body) {
		t.Fatalf("expected untouched body for non-codex format, got: %s", resp.Body)
	}
	// empty body -> passthrough without error
	raw, err = normalize(mk("muse-free", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); len(resp.Body) != 0 {
		t.Fatalf("expected empty passthrough, got: %s", resp.Body)
	}
	// ToFormat case/whitespace tolerant -> stripped
	raw, err = normalize(mkFormat(" Codex ", "muse-free", body))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); len(resp.Body) == 0 || strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("expected stripped body for case-insensitive codex, got: %s", resp.Body)
	}
	// uppercase thinking suffix -> stripped
	raw, err = normalize(mk("muse-free(HIGH)", body))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); len(resp.Body) == 0 || strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("expected stripped body for uppercase suffix, got: %s", resp.Body)
	}
	// double execution: intercept output fed into normalize stays stripped
	intercepted, err := intercept(func() []byte {
		raw, _ := json.Marshal(interceptRequest{SourceFormat: "codex", Model: "muse-free", RequestedModel: "muse-free", Body: body})
		return raw
	}())
	if err != nil {
		t.Fatal(err)
	}
	stripped := decodeIntercept(t, intercepted).Body
	if len(stripped) == 0 {
		t.Fatalf("expected intercept to strip first")
	}
	raw, err = normalize(mk("muse-free", stripped))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); strings.Contains(string(resp.Body), "additional_tools") {
		t.Fatalf("double execution reintroduced additional_tools: %s", resp.Body)
	}
	// outer invalid JSON -> error (host treats as skip, fail-open)
	if _, err := normalize([]byte(`{invalid`)); err == nil {
		t.Fatalf("expected error for invalid outer JSON")
	}
	// invalid inner body -> passthrough without error
	raw, err = normalize(mk("muse-free", []byte(`{invalid`)))
	if err != nil {
		t.Fatal(err)
	}
	if resp := decode(t, raw); string(resp.Body) != `{invalid` {
		t.Fatalf("expected passthrough for invalid body, got: %s", resp.Body)
	}
}

func TestRegisterCapabilities(t *testing.T) {
	raw, err := handleMethod("plugin.register", nil)
	if err != nil {
		t.Fatal(err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK {
		t.Fatalf("register not ok: %s", raw)
	}
	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatal(err)
	}
	if !reg.Capabilities.RequestInterceptor {
		t.Errorf("request_interceptor = false, want true")
	}
	if !reg.Capabilities.RequestNormalizer {
		t.Errorf("request_normalizer = false, want true")
	}
	if reg.Metadata.Version != pluginVersion {
		t.Errorf("metadata version = %q, want %q", reg.Metadata.Version, pluginVersion)
	}
}
