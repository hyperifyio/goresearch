package app

import (
    "archive/tar"
    "compress/gzip"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strings"
    "time"

    "github.com/hyperifyio/goresearch/internal/brief"
    "github.com/hyperifyio/goresearch/internal/planner"
    "github.com/hyperifyio/goresearch/internal/search"
    "github.com/hyperifyio/goresearch/internal/synth"
)

// exportArtifactsBundle writes a deterministic set of artifacts under
// ReportsDir/slug(topic)/ and optionally a tar.gz containing those files.
func exportArtifactsBundle(cfg Config, b brief.Brief, plan planner.Plan, selected []search.Result, excerpts []synth.SourceExcerpt, finalReportMarkdown string) error {
    root := strings.TrimSpace(cfg.ReportsDir)
    if root == "" {
        return nil
    }
    topic := strings.TrimSpace(b.Topic)
    if topic == "" {
        topic = "topic"
    }
    dir := filepath.Join(root, slugify(topic))
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return fmt.Errorf("mkdir bundle dir: %w", err)
    }

    // 1) planner.json
    if err := writeJSON(filepath.Join(dir, "planner.json"), plan); err != nil {
        return err
    }
    // 2) selected.json (stable order by URL)
    type selectedItem struct{ Title, URL, Snippet string }
    sel := make([]selectedItem, 0, len(selected))
    for _, r := range selected {
        sel = append(sel, selectedItem{Title: r.Title, URL: r.URL, Snippet: r.Snippet})
    }
    sort.Slice(sel, func(i, j int) bool { return strings.Compare(sel[i].URL, sel[j].URL) < 0 })
    if err := writeJSON(filepath.Join(dir, "selected.json"), sel); err != nil { return err }

    // 3) extracts.json (the exact excerpts used)
    if excerpts != nil {
        // Keep as-is order by Index for auditability
        if err := writeJSON(filepath.Join(dir, "extracts.json"), excerpts); err != nil { return err }
    }

    // 4) final report copy as report.md
    if strings.TrimSpace(finalReportMarkdown) != "" {
        if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(finalReportMarkdown), 0o644); err != nil {
            return fmt.Errorf("write report copy: %w", err)
        }
    }

    // 5) manifest.json — copy sidecar if present, else regenerate
    manSidecar := deriveManifestSidecarPath(cfg.OutputPath)
    if bts, err := os.ReadFile(manSidecar); err == nil && len(bts) > 0 {
        if err := os.WriteFile(filepath.Join(dir, "manifest.json"), bts, 0o644); err != nil {
            return fmt.Errorf("write manifest copy: %w", err)
        }
    } else if excerpts != nil {
        // regenerate with current meta
        meta := manifestMeta{
            Model:       cfg.LLMModel,
            LLMBaseURL:  cfg.LLMBaseURL,
            SourceCount: len(excerpts),
            HTTPCache:   true,
            LLMCache:    true,
            GeneratedAt: time.Now().UTC(),
        }
        entries := buildManifestEntriesFromSynth(excerpts)
        if data, mErr := marshalManifestJSON(meta, entries); mErr == nil {
            _ = os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644)
        }
    }

    // 6) evidence appendix as evidence.md if present
    if ev := extractEvidenceAppendix(finalReportMarkdown); strings.TrimSpace(ev) != "" {
        if err := os.WriteFile(filepath.Join(dir, "evidence.md"), []byte(ev), 0o644); err != nil {
            return fmt.Errorf("write evidence: %w", err)
        }
    }

    // 7) SHA256SUMS for all files in the bundle directory (excluding tarball)
    if err := writeSHA256SUMS(dir); err != nil {
        return err
    }

    // 8) Optional tar.gz archive of the directory
    if cfg.ReportsTar {
        tarPath := filepath.Join(root, slugify(topic)+".tar.gz")
        if err := tarGzDirectory(dir, tarPath); err != nil {
            return fmt.Errorf("tar bundle: %w", err)
        }
    }

    return nil
}

func slugify(s string) string {
    s = strings.ToLower(strings.TrimSpace(s))
    // Replace non-alphanumeric with hyphens
    re := regexp.MustCompile(`[^a-z0-9]+`)
    s = re.ReplaceAllString(s, "-")
    s = strings.Trim(s, "-")
    if s == "" { s = "topic" }
    return s
}

func writeJSON(path string, v any) error {
    b, err := json.MarshalIndent(v, "", "  ")
    if err != nil { return err }
    return os.WriteFile(path, b, 0o644)
}

func extractEvidenceAppendix(md string) string {
    md = strings.ReplaceAll(md, "\r\n", "\n")
    lines := strings.Split(md, "\n")
    start := -1
    for i, ln := range lines {
        s := strings.TrimSpace(ln)
        if !strings.HasPrefix(s, "## ") { continue }
        // Normalize heading text: strip leading '## ' and lowercase
        ht := strings.ToLower(strings.TrimSpace(s[3:]))
        // Allow optional "appendix X. " prefix before the title
        if strings.HasPrefix(ht, "appendix ") && len(ht) > len("appendix ")+2 {
            // drop up to the first '.' following the appendix label
            if dot := strings.Index(ht, ". "); dot != -1 {
                ht = strings.TrimSpace(ht[dot+2:])
            }
        }
        if strings.HasPrefix(ht, "evidence check") || strings.HasPrefix(ht, "evidence") {
            start = i
            break
        }
    }
    if start == -1 {
        return ""
    }
    // find next top-level section starting with '## ' after start
    end := len(lines)
    for i := start+1; i < len(lines); i++ {
        if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
            end = i
            break
        }
    }
    section := strings.Join(lines[start:end], "\n")
    if strings.TrimSpace(section) == "" { return "" }
    return section
}

func writeSHA256SUMS(dir string) error {
    entries, err := os.ReadDir(dir)
    if err != nil { return err }
    var b strings.Builder
    for _, e := range entries {
        if e.IsDir() { continue }
        name := e.Name()
        if strings.HasSuffix(name, ".tar.gz") { continue }
        p := filepath.Join(dir, name)
        sum, err := sha256File(p)
        if err != nil { return err }
        b.WriteString(sum)
        b.WriteString("  ")
        b.WriteString(name)
        b.WriteString("\n")
    }
    return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(b.String()), 0o644)
}

func sha256File(path string) (string, error) {
    f, err := os.Open(path)
    if err != nil { return "", err }
    defer f.Close()
    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil { return "", err }
    return hex.EncodeToString(h.Sum(nil)), nil
}

func tarGzDirectory(srcDir, outPath string) error {
    out, err := os.Create(outPath)
    if err != nil { return err }
    defer out.Close()
    gz := gzip.NewWriter(out)
    defer gz.Close()
    tw := tar.NewWriter(gz)
    defer tw.Close()

    base := filepath.Base(srcDir)
    return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
        if err != nil { return err }
        if info.IsDir() { return nil }
        rel, err := filepath.Rel(filepath.Dir(srcDir), path)
        if err != nil { return err }
        // Ensure the files are nested under base directory in the tar
        if !strings.HasPrefix(rel, base+string(os.PathSeparator)) {
            rel = filepath.Join(base, filepath.Base(path))
        }
        hdr, err := tar.FileInfoHeader(info, "")
        if err != nil { return err }
        hdr.Name = rel
        if err := tw.WriteHeader(hdr); err != nil { return err }
        f, err := os.Open(path)
        if err != nil { return err }
        if _, err := io.Copy(tw, f); err != nil {
            f.Close()
            return err
        }
        f.Close()
        return nil
    })
}
