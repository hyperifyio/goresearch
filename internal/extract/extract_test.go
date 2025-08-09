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

func TestFromHTML_BoilerplateReduction_CookieBanner(t *testing.T) {
    html := `<!doctype html>
    <html>
      <head><title>Cookie Page</title></head>
      <body>
        <main>
          <div id="cookie-banner">We use cookies to improve your experience.</div>
          <div class="cookie-consent">Accept all cookies</div>
          <p>Main content that should remain.</p>
        </main>
      </body>
    </html>`

    doc := FromHTML([]byte(html))
    if doc.Title != "Cookie Page" {
        t.Fatalf("expected title 'Cookie Page', got %q", doc.Title)
    }
    if !strings.Contains(doc.Text, "Main content that should remain.") {
        t.Fatalf("expected main content to be present; got: %q", doc.Text)
    }
    if strings.Contains(doc.Text, "We use cookies") || strings.Contains(doc.Text, "Accept all cookies") {
        t.Fatalf("expected cookie/consent banner text to be removed; got: %q", doc.Text)
    }
}

func TestFromHTML_BoilerplateReduction_DoesNotStripLegitimateCookieWord(t *testing.T) {
    html := `<!doctype html>
    <html>
      <head><title>Legit Cookie</title></head>
      <body>
        <article>
          <h2>Security</h2>
          <p>Cookie-based authentication is a common pattern.</p>
        </article>
      </body>
    </html>`

    doc := FromHTML([]byte(html))
    if doc.Title != "Legit Cookie" {
        t.Fatalf("expected title 'Legit Cookie', got %q", doc.Title)
    }
    if !strings.Contains(doc.Text, "Cookie-based authentication is a common pattern.") {
        t.Fatalf("expected sentence mentioning cookie to remain; got: %q", doc.Text)
    }
}

func TestNormalizeText_UnicodeWhitespaceAndDedupe(t *testing.T) {
    // Includes combining characters and multiple spaces, plus duplicate lines
    html := "<!doctype html>\n" +
        "<html>\n" +
        "  <head><title>NFC Test</title></head>\n" +
        "  <body>\n" +
        "    <main>\n" +
        "      <h1>Title</h1>\n" +
        "      <p>cafe\u0301 and caf\u00E9</p>\n" +
        "      <p>Repeat line</p>\n" +
        "      <p>Repeat   line</p>\n" +
        "    </main>\n" +
        "  </body>\n" +
        "</html>"

    doc := FromHTML([]byte(html))
    if doc.Title != "NFC Test" {
        t.Fatalf("expected title 'NFC Test', got %q", doc.Title)
    }
    // After Unicode normalization, both forms of "café" should be the same NFC form
    if !strings.Contains(doc.Text, "café") {
        t.Fatalf("expected NFC-normalized 'café' in text; got: %q", doc.Text)
    }
    // Duplicate lines "Repeat line" and "Repeat   line" should collapse and dedupe to a single line
    count := strings.Count(doc.Text, "Repeat line")
    if count != 1 {
        t.Fatalf("expected a single 'Repeat line' after collapse+dedupe, got count=%d; text=%q", count, doc.Text)
    }
}

func TestFromPDF_MinimalExtraction(t *testing.T) {
    // Simple fake PDF content stream with one text string
    pdf := "%PDF-1.4\n1 0 obj\n<</Type/Catalog>>\nendobj\nstream\n(Hello, PDF World!) Tj\n(Second line) Tj\nendstream\n%%EOF"
    doc := FromPDF([]byte(pdf))
    if doc.Text == "" {
        t.Fatalf("expected some text from PDF")
    }
    if !strings.Contains(doc.Text, "Hello, PDF World!") {
        t.Fatalf("expected first string extracted; got: %q", doc.Text)
    }
    if !strings.Contains(doc.Text, "Second line") {
        t.Fatalf("expected second string extracted; got: %q", doc.Text)
    }
}


