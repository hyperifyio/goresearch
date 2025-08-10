package app

import (
    "strconv"
    "strings"
    "time"
    "os"
)

// ApplyEnvToConfig populates unset fields of cfg from environment variables.
// Explicit cfg values take precedence over env.
func ApplyEnvToConfig(cfg *Config) {
    if cfg == nil { return }

    if cfg.LLMBaseURL == "" {
        cfg.LLMBaseURL = os.Getenv("LLM_BASE_URL")
    }
    if cfg.LLMModel == "" {
        cfg.LLMModel = os.Getenv("LLM_MODEL")
    }
    if cfg.LLMAPIKey == "" {
        cfg.LLMAPIKey = os.Getenv("LLM_API_KEY")
    }

    if cfg.SearxURL == "" {
        // Support both SEARX_URL and SEARXNG_URL; prefer SEARX_URL if set
        v := os.Getenv("SEARX_URL")
        if v == "" { v = os.Getenv("SEARXNG_URL") }
        cfg.SearxURL = v
    }
    if cfg.SearxKey == "" {
        v := os.Getenv("SEARX_KEY")
        if v == "" { v = os.Getenv("SEARXNG_KEY") }
        cfg.SearxKey = v
    }

    if cfg.CacheDir == "" {
        cfg.CacheDir = os.Getenv("CACHE_DIR")
    }

    if cfg.LanguageHint == "" {
        cfg.LanguageHint = os.Getenv("LANGUAGE")
    }

    // SOURCE_CAPS can be "<max>" or "<max>,<perDomain>"
    if cfg.MaxSources == 0 || cfg.PerDomainCap == 0 {
        caps := strings.TrimSpace(os.Getenv("SOURCE_CAPS"))
        if caps != "" {
            parts := strings.Split(caps, ",")
            if len(parts) >= 1 {
                if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && n > 0 && cfg.MaxSources == 0 {
                    cfg.MaxSources = n
                }
            }
            if len(parts) >= 2 {
                if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && n > 0 && cfg.PerDomainCap == 0 {
                    cfg.PerDomainCap = n
                }
            }
        }
    }

    // Optional durations
    if cfg.CacheMaxAge == 0 {
        if s := os.Getenv("CACHE_MAX_AGE"); s != "" {
            if d, err := time.ParseDuration(s); err == nil {
                cfg.CacheMaxAge = d
            }
        }
    }

    // Booleans
    setBool := func(dst *bool, envKey string) {
        if *dst { return }
        if s := strings.ToLower(strings.TrimSpace(os.Getenv(envKey))); s != "" {
            if s == "1" || s == "true" || s == "yes" || s == "on" {
                *dst = true
            }
        }
    }
    setBool(&cfg.DryRun, "DRY_RUN")
    setBool(&cfg.Verbose, "VERBOSE")
    setBool(&cfg.CacheClear, "CACHE_CLEAR")
    setBool(&cfg.CacheStrictPerms, "CACHE_STRICT_PERMS")
    setBool(&cfg.HTTPCacheOnly, "HTTP_CACHE_ONLY")
    setBool(&cfg.LLMCacheOnly, "LLM_CACHE_ONLY")
}

// ApplyEnvOverrides forcefully overrides cfg fields with environment variables
// when the corresponding env vars are set. This is used to let env take
// precedence over values coming from a config file while still allowing flags
// to remain highest precedence.
func ApplyEnvOverrides(cfg *Config) {
    if cfg == nil { return }

    if v := os.Getenv("LLM_BASE_URL"); v != "" { cfg.LLMBaseURL = v }
    if v := os.Getenv("LLM_MODEL"); v != "" { cfg.LLMModel = v }
    if v := os.Getenv("LLM_API_KEY"); v != "" { cfg.LLMAPIKey = v }

    if v := os.Getenv("SEARX_URL"); v != "" { cfg.SearxURL = v }
    if v := os.Getenv("SEARXNG_URL"); v != "" { cfg.SearxURL = v }
    if v := os.Getenv("SEARX_KEY"); v != "" { cfg.SearxKey = v }
    if v := os.Getenv("SEARXNG_KEY"); v != "" { cfg.SearxKey = v }

    if v := os.Getenv("CACHE_DIR"); v != "" { cfg.CacheDir = v }
    if v := os.Getenv("LANGUAGE"); v != "" { cfg.LanguageHint = v }

    if v := strings.TrimSpace(os.Getenv("SOURCE_CAPS")); v != "" {
        parts := strings.Split(v, ",")
        if len(parts) >= 1 {
            if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil && n > 0 {
                cfg.MaxSources = n
            }
        }
        if len(parts) >= 2 {
            if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && n > 0 {
                cfg.PerDomainCap = n
            }
        }
    }

    if s := os.Getenv("CACHE_MAX_AGE"); s != "" {
        if d, err := time.ParseDuration(s); err == nil {
            cfg.CacheMaxAge = d
        }
    }

    // Booleans override when env present and truthy/falsey
    setBool := func(dst *bool, envKey string) {
        if s := strings.ToLower(strings.TrimSpace(os.Getenv(envKey))); s != "" {
            switch s {
            case "1", "true", "yes", "on":
                *dst = true
            case "0", "false", "no", "off":
                *dst = false
            }
        }
    }
    setBool(&cfg.DryRun, "DRY_RUN")
    setBool(&cfg.Verbose, "VERBOSE")
    setBool(&cfg.CacheClear, "CACHE_CLEAR")
    setBool(&cfg.CacheStrictPerms, "CACHE_STRICT_PERMS")
    setBool(&cfg.HTTPCacheOnly, "HTTP_CACHE_ONLY")
    setBool(&cfg.LLMCacheOnly, "LLM_CACHE_ONLY")
}
