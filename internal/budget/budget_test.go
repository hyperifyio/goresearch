package budget

import "testing"

func TestEstimateTokensFromChars(t *testing.T) {
    cases := []struct{
        in int
        want int
    }{
        {0, 0},
        {1, 1},           // ceil(1/4)=1
        {3, 1},           // ceil(3/4)=1
        {4, 1},           // ceil(4/4)=1
        {5, 2},           // ceil(5/4)=2
        {400, 100},
    }
    for _, c := range cases {
        got := EstimateTokensFromChars(c.in)
        if got != c.want {
            t.Fatalf("EstimateTokensFromChars(%d) = %d, want %d", c.in, got, c.want)
        }
    }
}

func TestEstimatePromptTokens(t *testing.T) {
    sys := "system"
    user := "user message"
    ex := []string{"abc", "defg"}
    got := EstimatePromptTokens(sys, user, ex)
    // sys(6)->2, user(12)->3, ex: 3->1, 4->1 => total 7
    if got != 7 {
        t.Fatalf("EstimatePromptTokens() = %d, want %d", got, 7)
    }
}

func TestModelContextTokens(t *testing.T) {
    if ModelContextTokens("") != 8192 {
        t.Fatal("empty model should default to 8192")
    }
    if ModelContextTokens("gpt-4o") < 100_000 {
        t.Fatal("gpt-4o should be large (~128k)")
    }
    if ModelContextTokens("LLAMA-3.1") < 100_000 {
        t.Fatal("case-insensitive match for llama-3.1 should be ~128k")
    }
    if ModelContextTokens("mystery-512k") != 512_000 {
        t.Fatal("numeric suffix heuristic 512k should map to 512k tokens")
    }
}

func TestRemainingAndFits(t *testing.T) {
    model := "gpt-4o"
    max := ModelContextTokens(model)
    prompt := max / 2
    rem := RemainingContext(model, 2000, prompt)
    if rem <= 0 {
        t.Fatalf("remaining should be positive, got %d", rem)
    }
    if !FitsInContext(model, 2000, prompt) {
        t.Fatal("prompt should fit when remaining is positive")
    }
    // Force overflow
    prompt = max
    rem = RemainingContext(model, 1, prompt)
    if rem != 0 {
        t.Fatalf("remaining should clamp at 0 on overflow, got %d", rem)
    }
    if FitsInContext(model, 1, prompt) {
        t.Fatal("prompt should not fit when overflowed")
    }
}


