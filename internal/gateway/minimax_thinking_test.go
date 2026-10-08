package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Minimax-M3's content carries its thinking inline as a leading
// <think>…</think> (#1267): the tags and the whole thought reached the
// client as answer text, unfolded in Codex. The chat decoder passes it on
// as thinking, as it does AI Studio's <thought> spelling; only a tag at
// the very start of the reply is thinking, a later one stays text.
func TestChatThinkTags(t *testing.T) {
	run := func(parts ...string) (think, text string, order []EventKind) {
		var d chatDecoder
		for i, p := range parts {
			delta := map[string]any{"content": p}
			ch := map[string]any{"index": 0, "delta": delta}
			if i == len(parts)-1 {
				ch["finish_reason"] = "stop"
			}
			b, _ := json.Marshal(map[string]any{"id": "x", "choices": []any{ch}})
			d.decode(string(b), func(ev Event) {
				switch ev.Kind {
				case KThink:
					think += ev.Text
				case KText:
					text += ev.Text
				default:
					return
				}
				if len(order) == 0 || order[len(order)-1] != ev.Kind {
					order = append(order, ev.Kind)
				}
			})
		}
		return
	}
	for _, c := range []struct {
		name        string
		parts       []string
		think, text string
	}{
		{"whole", []string{"<think>Greeting.</think>Hello"}, "Greeting.", "Hello"},
		{"split tags", []string{"<thi", "nk>Let me", " think</thi", "nk>\n\nHello", " there"}, "Let me think", "Hello there"},
		{"one byte a chunk", strings.Split("<think>ab</think>cd", ""), "ab", "cd"},
		{"leading newline", []string{"\n", "<think>x</think>", "y"}, "x", "y"},
		{"a tag later", []string{"Hi ", "<think>no</think>"}, "", "Hi <think>no</think>"},
		{"never closed", []string{"<think>half", " a thought</th"}, "half a thought</th", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			think, text, order := run(c.parts...)
			if think != c.think || text != c.text {
				t.Errorf("think %q text %q, want %q %q", think, text, c.think, c.text)
			}
			if c.think != "" && c.text != "" && (len(order) != 2 || order[0] != KThink) {
				t.Errorf("order %v, want thinking then text", order)
			}
		})
	}
}

// Codex folds a Minimax-M3 thought into a reasoning item ahead of the
// answer, no tags left in what it shows.
func TestChatThinkTagsToResponses(t *testing.T) {
	rec := httptest.NewRecorder()
	enc := encoder("responses", newSSEWriter(rec), &Request{Model: "m"})
	var d chatDecoder
	for _, c := range []string{
		`{"id":"x","choices":[{"index":0,"delta":{"role":"assistant","content":"<think>Greeting"}}]}`,
		`{"id":"x","choices":[{"index":0,"delta":{"content":" the user.</think>Hello"}}]}`,
		`{"id":"x","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	} {
		d.decode(c, enc.event)
		if strings.Contains(c, "<think>Greeting") && !strings.Contains(rec.Body.String(), `"response.reasoning_summary_text.delta"`) {
			t.Fatalf("the first thought not sent at once:\n%s", rec.Body.String())
		}
	}
	enc.finish()
	body := rec.Body.String()
	rs, tx := strings.Index(body, `"type":"reasoning"`), strings.Index(body, `"type":"message"`)
	if rs < 0 || tx < 0 || rs > tx {
		t.Fatalf("want a reasoning item then a message item:\n%s", body)
	}
	if strings.Contains(body, "<think>") || strings.Contains(body, "</think>") {
		t.Fatalf("tags left in the reply:\n%s", body)
	}
}
