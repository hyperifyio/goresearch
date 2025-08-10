package app

import (
    "crypto/sha256"
    "encoding/hex"
    "path/filepath"
    "strings"

    "github.com/hyperifyio/goresearch/internal/brief"
)

// deriveReportsOutputPath returns a stable Markdown output path under the
// reports directory for the given brief topic. The filename uses a slugified
// topic and a short topic hash to ensure stability and avoid collisions.
func deriveReportsOutputPath(cfg Config, b brief.Brief) string {
    root := strings.TrimSpace(cfg.ReportsDir)
    if root == "" { root = "reports" }
    topic := strings.TrimSpace(b.Topic)
    if topic == "" { topic = "topic" }
    slug := slugify(topic)
    hash := strings.TrimSpace(cfg.TopicHash)
    if hash == "" {
        h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(topic))))
        hash = hex.EncodeToString(h[:])
    }
    // Use a short prefix of the hash for readability while remaining stable.
    short := hash
    if len(short) > 12 { short = short[:12] }
    name := slug + "-" + short + ".md"
    return filepath.Join(root, name)
}
