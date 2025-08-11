package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hyperifyio/goresearch/internal/search"
)

func main() {
	provider := &search.FileProvider{Path: "./fixtures/hello-search.json"}
	
	// Test query that should match
	query := "Hello Research — Brief introduction to goresearch specification"
	results, err := provider.Search(context.Background(), query, 10)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Query: %s\n", query)
	fmt.Printf("Results: %d\n", len(results))
	for i, r := range results {
		fmt.Printf("%d. %s\n", i+1, r.Title)
		fmt.Printf("   URL: %s\n", r.URL)
		fmt.Printf("   Snippet: %s\n", r.Snippet)
		fmt.Printf("\n")
	}
	
	// Also test with a simpler query
	fmt.Println("---")
	query2 := "hello"
	results2, err := provider.Search(context.Background(), query2, 10)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Query: %s\n", query2)
	fmt.Printf("Results: %d\n", len(results2))
	for i, r := range results2 {
		fmt.Printf("%d. %s\n", i+1, r.Title)
	}
}