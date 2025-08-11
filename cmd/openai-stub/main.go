package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

type chatRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func main() {
	model := os.Getenv("MODEL_ID")
	if strings.TrimSpace(model) == "" {
		model = "test-model"
	}
	addr := os.Getenv("ADDR")
	if strings.TrimSpace(addr) == "" {
		addr = ":8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": model, "object": "model"}},
		})
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		sys := ""
		if len(req.Messages) > 0 {
			sys = strings.TrimSpace(req.Messages[0].Content)
		}
		var content string
		switch {
		case strings.Contains(sys, "Respond with strict JSON only") && strings.Contains(sys, "queries"):
			// Planner
			plan := map[string]any{
				"queries": []string{
					"System Test specification",
					"System Test documentation",
					"System Test reference",
					"System Test tutorial",
					"System Test best practices",
					"System Test faq",
					"System Test examples",
					"System Test limitations",
				},
				"outline": []string{"Executive summary", "Background", "Core concepts", "Implementation guidance", "Alternatives & conflicting evidence", "Examples", "Risks and limitations"},
			}
			b, _ := json.Marshal(plan)
			content = string(b)
		case strings.Contains(sys, "careful technical writer"):
			user := ""
			if len(req.Messages) >= 2 {
				user = req.Messages[1].Content
			}
			urls := make([]string, 0, 8)
			for _, line := range strings.Split(user, "\n") {
				line = strings.TrimSpace(line)
				if len(line) > 2 && line[0] >= '0' && line[0] <= '9' && strings.Contains(line, " — ") {
					parts := strings.SplitN(line, " — ", 2)
					if len(parts) == 2 {
						url := strings.TrimSpace(parts[1])
						urls = append(urls, url)
					}
				}
			}
			ref1, ref2 := "https://example.com/a", "https://example.com/b"
			if len(urls) >= 1 {
				ref1 = urls[0]
			}
			if len(urls) >= 2 {
				ref2 = urls[1]
			}
			content = "# System Test Report\n2025-01-01\n\n## Executive summary\nShort summary citing [1].\n\n## Background\nContext with refs [1][2].\n\n## Alternatives & conflicting evidence\nBrief alternatives [1].\n\n## Risks and limitations\nSome cautions.\n\n## References\n1. One — " + ref1 + "\n2. Two — " + ref2 + "\n\n## Evidence check\nOK."
		case strings.Contains(sys, "fact-check verifier"):
			res := map[string]any{
				"claims": []map[string]any{
					{"text": "Claim with [1]", "citations": []int{1}, "confidence": "medium", "supported": true},
				},
				"summary": "1 claim supported.",
			}
			b, _ := json.Marshal(res)
			content = string(b)
		default:
			http.Error(w, "unexpected system", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": content}},
			},
		})
	})

	log.Printf("openai-stub listening on %s (model=%s)", addr, model)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
