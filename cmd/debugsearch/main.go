package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/hyperifyio/goresearch/internal/search"
)

func main() {
	base := os.Getenv("SEARX_URL")
	if base == "" { base = "http://localhost:8888" }
	q := "What is love?"
	if len(os.Args) > 1 { q = os.Args[1] }
	client := &http.Client{ Timeout: 20 * time.Second }
	prov := &search.SearxNG{BaseURL: base, HTTPClient: client, UserAgent: "debugsearch/1.0"}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	res, err := prov.Search(ctx, q, 5)
	fmt.Println("err:", err)
	for i, r := range res {
		fmt.Printf("%d. %s — %s\n", i+1, r.Title, r.URL)
	}
}
