package app

import (
    "bufio"
    "errors"
    "os"
    "strings"
)

// LoadEnvFiles loads one or more dotenv files of KEY=VALUE pairs into the
// process environment. Later files override earlier ones. Lines starting with
// '#' and blank lines are ignored. Values are not expanded.
func LoadEnvFiles(paths ...string) error {
    for _, p := range paths {
        if strings.TrimSpace(p) == "" {
            continue
        }
        if err := loadEnvFile(p); err != nil {
            // Missing files are not fatal; continue to next path
            if errors.Is(err, os.ErrNotExist) {
                continue
            }
            return err
        }
    }
    return nil
}

func loadEnvFile(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        // Simple KEY=VALUE parser; stops at first '='
        eq := strings.IndexByte(line, '=')
        if eq <= 0 {
            // ignore malformed lines silently
            continue
        }
        key := strings.TrimSpace(line[:eq])
        val := strings.TrimSpace(line[eq+1:])
        // strip optional surrounding quotes
        val = strings.Trim(val, " \t\r\n")
        if len(val) >= 2 {
            if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
                val = val[1 : len(val)-1]
            }
        }
        _ = os.Setenv(key, val)
    }
    return scanner.Err()
}
