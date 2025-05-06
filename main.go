package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"./algorithms"
	"./scraper"
)

type SearchRequest struct {
    Target    string `json:"target"`
    Algorithm string `json:"algorithm"`
}

func main() {
    http.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
        if _, err := os.Stat("little_alchemy_elements.json"); os.IsNotExist(err) {
            fmt.Println("Scraping...")
            if err := scraper.Run(); err != nil {
                http.Error(w, "Scraping failed", 500)
                return
            }
        }

        var req SearchRequest
        json.NewDecoder(r.Body).Decode(&req)

        elements, err := scraper.LoadData()
        if err != nil {
            http.Error(w, "Failed to load data", 500)
            return
        }

        var steps []string
        switch req.Algorithm {
        case "bfs":
            steps = algorithms.BFS(req.Target, elements)
        case "dfs":
            steps = algorithms.DFS(req.Target, elements)
        case "bidirectional":
            steps = algorithms.Bidirectional(req.Target, elements)
        default:
            http.Error(w, "Invalid algorithm", 400)
            return
        }

        json.NewEncoder(w).Encode(steps)
    })

    fmt.Println("Server listening on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
