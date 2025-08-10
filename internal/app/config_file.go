package app

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "time"

    yaml "gopkg.in/yaml.v3"
)

// FileConfig represents the single-file configuration schema.
// Nested sections improve readability and map naturally to flags/env.
type FileConfig struct {
    Input   string `yaml:"input" json:"input"`
    Output  string `yaml:"output" json:"output"`
    OutputPDF string `yaml:"outputPDF" json:"outputPDF"`

    LLM struct {
        BaseURL string `yaml:"base" json:"base"`
        Model   string `yaml:"model" json:"model"`
        APIKey  string `yaml:"key" json:"key"`
    } `yaml:"llm" json:"llm"`

    Searx struct {
        URL string `yaml:"url" json:"url"`
        Key string `yaml:"key" json:"key"`
        UA  string `yaml:"ua" json:"ua"`
    } `yaml:"searx" json:"searx"`

    Search struct {
        File string `yaml:"file" json:"file"`
    } `yaml:"search" json:"search"`

    Max struct {
        Sources        int `yaml:"sources" json:"sources"`
        PerDomain      int `yaml:"perDomain" json:"perDomain"`
        PerSourceChars int `yaml:"perSourceChars" json:"perSourceChars"`
    } `yaml:"max" json:"max"`

    Min struct {
        SnippetChars int `yaml:"snippetChars" json:"snippetChars"`
    } `yaml:"min" json:"min"`

    Language string `yaml:"language" json:"language"`
    DryRun   bool   `yaml:"dryRun" json:"dryRun"`
    Verbose  bool   `yaml:"verbose" json:"verbose"`
    DebugVerbose bool `yaml:"debugVerbose" json:"debugVerbose"`
    Verify   *struct{
        Enable *bool `yaml:"enable" json:"enable"`
    } `yaml:"verify" json:"verify"`

    Cache struct {
        Dir         string        `yaml:"dir" json:"dir"`
        MaxAge      time.Duration `yaml:"maxAge" json:"maxAge"`
        Clear       bool          `yaml:"clear" json:"clear"`
        StrictPerms bool          `yaml:"strictPerms" json:"strictPerms"`
        TopicHash   string        `yaml:"topicHash" json:"topicHash"`
    } `yaml:"cache" json:"cache"`

    EnablePDF bool `yaml:"enablePDF" json:"enablePDF"`

    Distribution struct {
        Enable   bool   `yaml:"enable" json:"enable"`
        Author   string `yaml:"author" json:"author"`
        Version  string `yaml:"version" json:"version"`
    } `yaml:"distribution" json:"distribution"`

    Prompts struct {
        SynthSystemPrompt      string `yaml:"synthSystemPrompt" json:"synthSystemPrompt"`
        SynthSystemPromptFile  string `yaml:"synthSystemPromptFile" json:"synthSystemPromptFile"`
        VerifySystemPrompt     string `yaml:"verifySystemPrompt" json:"verifySystemPrompt"`
        VerifySystemPromptFile string `yaml:"verifySystemPromptFile" json:"verifySystemPromptFile"`
    } `yaml:"prompts" json:"prompts"`

    Robots struct {
        OverrideDomains []string `yaml:"overrideDomains" json:"overrideDomains"`
        OverrideConfirm bool     `yaml:"overrideConfirm" json:"overrideConfirm"`
    } `yaml:"robots" json:"robots"`

    Domains struct {
        Allow []string `yaml:"allow" json:"allow"`
        Deny  []string `yaml:"deny" json:"deny"`
    } `yaml:"domains" json:"domains"`

    Tools struct {
        Enable           bool          `yaml:"enable" json:"enable"`
        DryRun           bool          `yaml:"dryRun" json:"dryRun"`
        MaxCalls         int           `yaml:"maxCalls" json:"maxCalls"`
        MaxWallClock     time.Duration `yaml:"maxWallClock" json:"maxWallClock"`
        PerToolTimeout   time.Duration `yaml:"perToolTimeout" json:"perToolTimeout"`
        Mode             string        `yaml:"mode" json:"mode"`
    } `yaml:"tools" json:"tools"`
}

// LoadConfigFile reads YAML or JSON into FileConfig.
func LoadConfigFile(path string) (FileConfig, error) {
    var fc FileConfig
    b, err := os.ReadFile(path)
    if err != nil {
        return fc, err
    }
    switch ext := filepath.Ext(path); ext {
    case ".yaml", ".yml":
        if err := yaml.Unmarshal(b, &fc); err != nil {
            return fc, fmt.Errorf("parse yaml: %w", err)
        }
    case ".json":
        if err := json.Unmarshal(b, &fc); err != nil {
            return fc, fmt.Errorf("parse json: %w", err)
        }
    default:
        // Try YAML then JSON
        if err := yaml.Unmarshal(b, &fc); err != nil {
            if jerr := json.Unmarshal(b, &fc); jerr != nil {
                return fc, fmt.Errorf("parse config: %v (yaml) / %v (json)", err, jerr)
            }
        }
    }
    return fc, nil
}

// ApplyFileConfig overlays values from FileConfig into cfg for any fields that
// are currently unset/zero in cfg. Flags should already have been parsed; this
// function lets file config supply defaults while preserving explicit flags.
func ApplyFileConfig(cfg *Config, fc FileConfig) {
    if cfg == nil { return }
    // Defaults from flag parsing that file config may override when flags not set
    const (
        inputDefault             = "request.md"
        outputDefault            = "report.md"
        searxUADefault           = "goresearch/1.0 (+https://github.com/hyperifyio/goresearch)"
        cacheDirDefault          = ".goresearch-cache"
        maxSourcesDefault        = 12
        perDomainDefault         = 3
        perSourceCharsDefault    = 12000
        minSnippetCharsDefault   = 0
        toolsMaxCallsDefault     = 32
        toolsPerToolTimeoutSecs  = 10
        toolsModeDefault         = "harmony"
    )

    if (cfg.InputPath == "" || cfg.InputPath == inputDefault) && fc.Input != "" { cfg.InputPath = fc.Input }
    if (cfg.OutputPath == "" || cfg.OutputPath == outputDefault) && fc.Output != "" { cfg.OutputPath = fc.Output }
    if fc.OutputPDF != "" { cfg.OutputPDFPath = fc.OutputPDF }

    if cfg.LLMBaseURL == "" && fc.LLM.BaseURL != "" { cfg.LLMBaseURL = fc.LLM.BaseURL }
    if cfg.LLMModel == "" && fc.LLM.Model != "" { cfg.LLMModel = fc.LLM.Model }
    if cfg.LLMAPIKey == "" && fc.LLM.APIKey != "" { cfg.LLMAPIKey = fc.LLM.APIKey }

    if cfg.SearxURL == "" && fc.Searx.URL != "" { cfg.SearxURL = fc.Searx.URL }
    if cfg.SearxKey == "" && fc.Searx.Key != "" { cfg.SearxKey = fc.Searx.Key }
    if (cfg.SearxUA == "" || cfg.SearxUA == searxUADefault) && fc.Searx.UA != "" { cfg.SearxUA = fc.Searx.UA }
    if cfg.FileSearchPath == "" && fc.Search.File != "" { cfg.FileSearchPath = fc.Search.File }

    if (cfg.MaxSources == 0 || cfg.MaxSources == maxSourcesDefault) && fc.Max.Sources > 0 { cfg.MaxSources = fc.Max.Sources }
    if (cfg.PerDomainCap == 0 || cfg.PerDomainCap == perDomainDefault) && fc.Max.PerDomain > 0 { cfg.PerDomainCap = fc.Max.PerDomain }
    if (cfg.PerSourceChars == 0 || cfg.PerSourceChars == perSourceCharsDefault) && fc.Max.PerSourceChars > 0 { cfg.PerSourceChars = fc.Max.PerSourceChars }
    if (cfg.MinSnippetChars == 0 || cfg.MinSnippetChars == minSnippetCharsDefault) && fc.Min.SnippetChars > 0 { cfg.MinSnippetChars = fc.Min.SnippetChars }
    if cfg.LanguageHint == "" && fc.Language != "" { cfg.LanguageHint = fc.Language }
    if !cfg.DryRun && fc.DryRun { cfg.DryRun = true }
    if !cfg.Verbose && fc.Verbose { cfg.Verbose = true }
    if !cfg.DebugVerbose && fc.DebugVerbose { cfg.DebugVerbose = true }
    // Verification toggle: default on; allow file config to disable when enable=false
    if fc.Verify != nil && fc.Verify.Enable != nil {
        if !*fc.Verify.Enable {
            cfg.DisableVerify = true
        } else {
            cfg.DisableVerify = false
        }
    }

    if (cfg.CacheDir == "" || cfg.CacheDir == cacheDirDefault) && fc.Cache.Dir != "" { cfg.CacheDir = fc.Cache.Dir }
    if cfg.CacheMaxAge == 0 && fc.Cache.MaxAge > 0 { cfg.CacheMaxAge = fc.Cache.MaxAge }
    if !cfg.CacheClear && fc.Cache.Clear { cfg.CacheClear = true }
    if !cfg.CacheStrictPerms && fc.Cache.StrictPerms { cfg.CacheStrictPerms = true }
    if cfg.TopicHash == "" && fc.Cache.TopicHash != "" { cfg.TopicHash = fc.Cache.TopicHash }

    if !cfg.EnablePDF && fc.EnablePDF { cfg.EnablePDF = true }
    if !cfg.DistributionChecks && fc.Distribution.Enable { cfg.DistributionChecks = true }
    if cfg.ExpectedAuthor == "" && fc.Distribution.Author != "" { cfg.ExpectedAuthor = fc.Distribution.Author }
    if cfg.ExpectedVersion == "" && fc.Distribution.Version != "" { cfg.ExpectedVersion = fc.Distribution.Version }

    if cfg.SynthSystemPrompt == "" && fc.Prompts.SynthSystemPrompt != "" { cfg.SynthSystemPrompt = fc.Prompts.SynthSystemPrompt }
    if cfg.VerifySystemPrompt == "" && fc.Prompts.VerifySystemPrompt != "" { cfg.VerifySystemPrompt = fc.Prompts.VerifySystemPrompt }

    if len(cfg.RobotsOverrideAllowlist) == 0 && len(fc.Robots.OverrideDomains) > 0 { cfg.RobotsOverrideAllowlist = append([]string{}, fc.Robots.OverrideDomains...) }
    if !cfg.RobotsOverrideConfirm && fc.Robots.OverrideConfirm { cfg.RobotsOverrideConfirm = true }

    if len(cfg.DomainAllowlist) == 0 && len(fc.Domains.Allow) > 0 { cfg.DomainAllowlist = append([]string{}, fc.Domains.Allow...) }
    if len(cfg.DomainDenylist) == 0 && len(fc.Domains.Deny) > 0 { cfg.DomainDenylist = append([]string{}, fc.Domains.Deny...) }

    if !cfg.ToolsEnabled && fc.Tools.Enable { cfg.ToolsEnabled = true }
    if !cfg.ToolsDryRun && fc.Tools.DryRun { cfg.ToolsDryRun = true }
    if (cfg.ToolsMaxCalls == 0 || cfg.ToolsMaxCalls == toolsMaxCallsDefault) && fc.Tools.MaxCalls > 0 { cfg.ToolsMaxCalls = fc.Tools.MaxCalls }
    if cfg.ToolsMaxWallClock == 0 && fc.Tools.MaxWallClock > 0 { cfg.ToolsMaxWallClock = fc.Tools.MaxWallClock }
    if (cfg.ToolsPerToolTimeout == 0 || cfg.ToolsPerToolTimeout == toolsPerToolTimeoutSecs*time.Second) && fc.Tools.PerToolTimeout > 0 { cfg.ToolsPerToolTimeout = fc.Tools.PerToolTimeout }
    if (cfg.ToolsMode == "" || cfg.ToolsMode == toolsModeDefault) && fc.Tools.Mode != "" { cfg.ToolsMode = fc.Tools.Mode }
}

// ValidateConfig performs minimal schema validation for required settings.
// For dry-run, LLM settings may be omitted.
func ValidateConfig(cfg Config) error {
    if trim(cfg.InputPath) == "" {
        return errors.New("config: input path is required")
    }
    if trim(cfg.OutputPath) == "" {
        return errors.New("config: output path is required")
    }
    if !cfg.DryRun {
        if trim(cfg.LLMModel) == "" {
            return errors.New("config: llm.model is required (or set LLM_MODEL)")
        }
    }
    if cfg.MaxSources < 0 || cfg.PerDomainCap < 0 || cfg.PerSourceChars < 0 {
        return errors.New("config: negative limits are not allowed")
    }
    return nil
}

func trim(s string) string {
    i := 0
    j := len(s)
    for i < j && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') { i++ }
    for j > i && (s[j-1] == ' ' || s[j-1] == '\t' || s[j-1] == '\n' || s[j-1] == '\r') { j-- }
    return s[i:j]
}
