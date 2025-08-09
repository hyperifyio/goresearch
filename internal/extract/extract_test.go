package extract

import (
    "strings"
    "testing"
)

func TestFromHTML_PrefersMainOverBody(t *testing.T) {
    html := `<!doctype html>
    <html>
      <head><title>Test Page</title></head>
      <body>
        <nav>Nav should be ignored</nav>
        <main>
          <h1>Main Heading</h1>
          <p>This is the main content paragraph.</p>
        </main>
        <footer>Footer text</footer>
      </body>
    </html>`

    doc := FromHTML([]byte(html))
    if doc.Title != "Test Page" {
        t.Fatalf("expected title 'Test Page', got %q", doc.Title)
    }
    if !strings.Contains(doc.Text, "Main Heading") {
        t.Fatalf("expected to contain main heading")
    }
    if !strings.Contains(doc.Text, "This is the main content paragraph.") {
        t.Fatalf("expected to contain main paragraph")
    }
    if strings.Contains(doc.Text, "Nav should be ignored") {
        t.Fatalf("did not expect nav text in extracted content")
    }
    if strings.Contains(doc.Text, "Footer text") {
        t.Fatalf("did not expect footer text in extracted content")
    }
}

func TestFromHTML_FallbackToBody(t *testing.T) {
    html := `<!doctype html>
    <html>
      <head><title>No Main</title></head>
      <body>
        <h2>Body Heading</h2>
        <p>Body paragraph</p>
      </body>
    </html>`

    doc := FromHTML([]byte(html))
    if doc.Title != "No Main" {
        t.Fatalf("expected title 'No Main', got %q", doc.Title)
    }
    if !strings.Contains(doc.Text, "Body Heading") {
        t.Fatalf("expected to contain body heading")
    }
    if !strings.Contains(doc.Text, "Body paragraph") {
        t.Fatalf("expected to contain body paragraph")
    }
}

func TestFromHTML_PreservesCodeAndListItems(t *testing.T) {
    html := `<!doctype html>
    <html>
      <head><title>Code and List</title></head>
      <body>
        <article>
          <h3>Examples</h3>
          <ul>
            <li>First item</li>
            <li>Second item</li>
          </ul>
          <pre><code>print("hello")\nprint("world")</code></pre>
        </article>
      </body>
    </html>`

    doc := FromHTML([]byte(html))
    if doc.Title != "Code and List" {
        t.Fatalf("expected title 'Code and List', got %q", doc.Title)
    }
    // list items appear in the text
    if !strings.Contains(doc.Text, "First item") || !strings.Contains(doc.Text, "Second item") {
        t.Fatalf("expected to contain list items; got: %q", doc.Text)
    }
    // code content is preserved verbatim
    if !strings.Contains(doc.Text, "print(\"hello\")") || !strings.Contains(doc.Text, "print(\"world\")") {
        t.Fatalf("expected code block content to be preserved; got: %q", doc.Text)
    }
}


