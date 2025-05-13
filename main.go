package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/farrelathalla/Tubes2_BE_Ahsan-geming/algorithms"
	"github.com/farrelathalla/Tubes2_BE_Ahsan-geming/scraper"
	"github.com/gorilla/websocket"
)

// SearchRequest represents a search request from the client
type SearchRequest struct {
	Target    string `json:"target"`
	Algorithm string `json:"algorithm"`
	Mode      string `json:"mode"`      // "shortest" or "multiple"
	Limit     int    `json:"limit,omitempty"` // Only for multiple mode
}

// ProcessUpdate represents an update in the algorithm process
type ProcessUpdate struct {
	Type     string      `json:"type"`     // "progress" or "result"
	Element  string      `json:"element"`  // Current element being processed
	Path     interface{} `json:"path"`     // Current path being built
	Complete bool        `json:"complete"` // Is this the final result?
	Stats    struct {
		NodeCount int `json:"nodeCount"`
		StepCount int `json:"stepCount"`
	} `json:"stats"`
}

// Connection manager
type ConnectionManager struct {
	connections map[*websocket.Conn]bool
	mutex       sync.Mutex
}

// WebSocket upgrader
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

var manager = ConnectionManager{
	connections: make(map[*websocket.Conn]bool),
}

func (m *ConnectionManager) Add(conn *websocket.Conn) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.connections[conn] = true
}

func (m *ConnectionManager) Remove(conn *websocket.Conn) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.connections, conn)
}

// Convert ElementData slice to recipe map format expected by algorithms
func convertToRecipeMap(elements []scraper.ElementData) map[string][][]string {
	recipeMap := make(map[string][][]string)
	
	for _, elem := range elements {
		var recipes [][]string
		
		for _, recipeStr := range elem.Recipes {
			// Handle potential variations in recipe format
			components := strings.Split(recipeStr, "+")
			
			// Clean up component names
			var cleanComponents []string
			for _, comp := range components {
				comp = strings.TrimSpace(comp)
				if comp != "" {
					cleanComponents = append(cleanComponents, comp)
				}
			}
			
			if len(cleanComponents) > 0 {
				recipes = append(recipes, cleanComponents)
			}
		}
		
		recipeMap[elem.Element] = recipes
	}
	
	return recipeMap
}

// Load elements data, running scraper if necessary
func loadElementsData() (map[string][][]string, map[string]int, error) {
	// Check if data file exists
	if _, err := os.Stat("little_alchemy_elements.json"); os.IsNotExist(err) {
		fmt.Println("Alchemy elements data file not found. Running scraper...")
		if err := scraper.Run(); err != nil {
			return nil, nil, fmt.Errorf("failed to run scraper: %w", err)
		}
		fmt.Println("Scraper completed successfully.")
	}
	
	// Load data
	elements, err := scraper.LoadData()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load elements data: %w", err)
	}
	
	// Convert to recipe map format
	recipeMap := convertToRecipeMap(elements)
	fmt.Printf("Loaded %d elements with recipes\n", len(recipeMap))
	
	// Load raw data to build tier map
	file, err := os.Open("little_alchemy_elements.json")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open elements data: %w", err)
	}
	defer file.Close()
	
	// Parse raw data
	var rawData map[string][]struct {
		Element string   `json:"element"`
		Recipes []string `json:"recipes"`
	}
	if err := json.NewDecoder(file).Decode(&rawData); err != nil {
		return nil, nil, fmt.Errorf("failed to parse elements data: %w", err)
	}
	
	// Build tier map
	tierMap := algorithms.BuildTierMap(rawData)
	
	return recipeMap, tierMap, nil
}

// WebSocket handler
func wsHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Add connection to manager
	manager.Add(conn)
	defer manager.Remove(conn)

	// Listen for messages from the client
	for {
		var req SearchRequest
		err := conn.ReadJSON(&req)
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}

		log.Printf("Received search via WebSocket: target=%s, algorithm=%s, mode=%s", 
			req.Target, req.Algorithm, req.Mode)

		// Process the request asynchronously
		go processSearch(conn, req)
	}
}

// Process search request
func processSearch(conn *websocket.Conn, req SearchRequest) {
	// Load elements data
	recipeMap, tierMap, err := loadElementsData()
	if err != nil {
		sendErrorMessage(conn, fmt.Sprintf("Failed to load data: %v", err))
		return
	}

	if req.Algorithm == "dfs" {
		if req.Mode == "shortest" {
			// Single recipe DFS with updates
			algorithms.DFSSingleWithUpdates(req.Target, recipeMap, tierMap, conn)
		} else if req.Mode == "multiple" {
			// Multiple recipe DFS with updates
			limit := 5 // Default
			if req.Limit > 0 {
				limit = req.Limit
			}
			algorithms.DFSMultipleWithUpdates(req.Target, recipeMap, tierMap, limit, conn)
		} else {
			sendErrorMessage(conn, "Invalid mode. Use 'shortest' or 'multiple'")
			return
		}
		// Note: DFS functions now send their own final results, similar to BFS functions
	} else if req.Algorithm == "bfs" {
		if req.Mode == "shortest" {
			// Single recipe BFS with updates
			algorithms.BFSSingleWithUpdates(req.Target, recipeMap, tierMap, conn)
		} else if req.Mode == "multiple" {
			// Multiple recipe BFS with updates
			limit := 5 // Default
			if req.Limit > 0 {
				limit = req.Limit
			}
			algorithms.BFSMultipleWithUpdates(req.Target, recipeMap, tierMap, limit, conn)
		} else {
			sendErrorMessage(conn, "Invalid mode. Use 'shortest' or 'multiple'")
			return
		}
		// Note: BFS functions send their own final results, so we don't need to here
	} else if req.Algorithm == "bidirectional" {
		if req.Mode == "shortest" {
			// Single recipe bidirectional with updates
			algorithms.SafeBidirectionalSingleWithUpdates(req.Target, recipeMap, tierMap, conn)
		} else if req.Mode == "multiple" {
			// Multiple recipe bidirectional with updates
			limit := 5 // Default
			if req.Limit > 0 {
				limit = req.Limit
			}
			algorithms.SafeBidirectionalMultipleWithUpdates(req.Target, recipeMap, tierMap, limit, conn)
		} else {
			sendErrorMessage(conn, "Invalid mode. Use 'shortest' or 'multiple'")
			return
		}
		// Note: Bidirectional functions send their own final results, similar to BFS functions
	} else {
		sendErrorMessage(conn, "Invalid algorithm. Use 'dfs', 'bfs', or 'bidirectional'")
		return
	}
}

// Send error message to client
func sendErrorMessage(conn *websocket.Conn, message string) {
	update := ProcessUpdate{
		Type:     "error",
		Element:  "",
		Path:     map[string]string{"error": message},
		Complete: true,
		Stats: struct {
			NodeCount int `json:"nodeCount"`
			StepCount int `json:"stepCount"`
		}{0, 0},
	}
	
	err := conn.WriteJSON(update)
	if err != nil {
		log.Printf("Error sending error message: %v", err)
	}
}

// Regular HTTP handler for search (backward compatibility)
func searchHandler(w http.ResponseWriter, r *http.Request) {
	// Allow cross-origin requests
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusOK)
		return
	}

	var req SearchRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		fmt.Println("Failed to parse request body:", err)
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	log.Printf("Received search: target=%s, algorithm=%s\n", req.Target, req.Algorithm)

	// For backward compatibility, provide simple responses
	// Note: The HTTP API is kept for backward compatibility but only provides basic responses
	switch req.Algorithm {
	case "dfs":
		// Simple response for HTTP API
		result := map[string]interface{}{
			"algorithm": "dfs",
			"message":   "DFS search completed. Use WebSocket for detailed results with live updates.",
			"target":    req.Target,
		}
		json.NewEncoder(w).Encode(result)
	case "bfs":
		// Simple response for HTTP API
		result := map[string]interface{}{
			"algorithm": "bfs",
			"message":   "BFS search completed. Use WebSocket for detailed results with live updates.",
			"target":    req.Target,
		}
		json.NewEncoder(w).Encode(result)
	case "bidirectional":
		// Simple response for HTTP API
		result := map[string]interface{}{
			"algorithm": "bidirectional",
			"message":   "Bidirectional search completed. Use WebSocket for detailed results with live updates.",
			"target":    req.Target,
		}
		json.NewEncoder(w).Encode(result)
	default:
		http.Error(w, "Invalid algorithm", http.StatusBadRequest)
		return
	}
}

func main() {
	// Set up HTTP routes
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/ws", wsHandler)
	
	// Serve static files
	fs := http.FileServer(http.Dir("./public"))
	http.Handle("/", fs)

	// Start the server
	// port := "8080"
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default for local development
	}
	fmt.Printf("Server listening on http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}