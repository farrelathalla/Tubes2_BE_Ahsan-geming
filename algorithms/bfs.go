package algorithms

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/gorilla/websocket"
)

type ProcessUpdate struct {
	Type     string      `json:"type"`     // "progress" or "result"
	Element  string      `json:"element"`  // Current element being processed
	Path     interface{} `json:"path"`     // Current path being built
	Complete bool        `json:"complete"` // Is this the final result?
	Stats    struct {
		NodeCount     int           `json:"nodeCount"`
		StepCount     int           `json:"stepCount"`
		ElapsedTime   time.Duration `json:"elapsedTime"`
		ElapsedTimeMs int64         `json:"elapsedTimeMs"` // For JSON serialization
	} `json:"stats"`
}

// Basic elements in the game
var startingElements = map[string]bool{
	"Water": true,
	"Earth": true,
	"Fire":  true,
	"Air":   true,
}

// RecipeNode represents a node in the recipe tree
type RecipeNode struct {
	Element string        `json:"element"`
	Recipes []*RecipeNode `json:"recipes"`
}

// ElementToProcess represents an element in the BFS queue
type ElementToProcess struct {
	Element string
	Depth   int
}

// RecipePath represents a path to an element
type RecipePath struct {
	Elements []string
	Recipes  map[string][]string
}

// RecipeInfo contains information about a recipe
type RecipeInfo struct {
	Recipe    []string
	TotalTier int
}

// WorkItem represents a work item for the BFS worker
type WorkItem struct {
	Path  RecipePath
	Queue []ElementToProcess
}

// WebSocketClient wraps a WebSocket connection with thread-safe methods
type WebSocketClient struct {
	conn *websocket.Conn
	mu   sync.Mutex
}
var totalNodeCounted int64
var totalNodeVisited int64

// SendUpdate sends an update to the client
func (c *WebSocketClient) SendUpdate(update ProcessUpdate) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(update)
}

// Helper to get valid recipes with lower tiers
func getValidRecipes(recipeMap map[string][][]string, tierMap map[string]int, element string) []RecipeInfo {
	recipes, found := recipeMap[element]
	if !found {
		return []RecipeInfo{}
	}

	elementTier, exists := tierMap[element]
	if !exists {
		elementTier = 999
	}

	validRecipes := []RecipeInfo{}

	for _, recipe := range recipes {
		totalTier := 0
		allLowerOrEqual := true

		for _, ing := range recipe {
			atomic.AddInt64(&totalNodeCounted, 1)
			ingTier, exists := tierMap[ing]
			if !exists {
				ingTier = 999
			}

			if ingTier >= elementTier {
				allLowerOrEqual = false
				break
			}

			totalTier += ingTier
		}

		if allLowerOrEqual {
			validRecipes = append(validRecipes, RecipeInfo{Recipe: recipe, TotalTier: totalTier})
		}
	}

	return validRecipes
}

// BuildTierMap builds a map of tiers from the raw data
func BuildTierMap(data map[string][]struct {
	Element string   `json:"element"`
	Recipes []string `json:"recipes"`
}) map[string]int {
	tierMap := make(map[string]int)

	for tierName, elements := range data {
		var tierLevel int
		if strings.HasPrefix(tierName, "Starting ") {
			tierLevel = 0
		} else if strings.HasPrefix(tierName, "Tier ") {
			_, err := fmt.Sscanf(tierName, "Tier %d", &tierLevel)
			if err != nil {
				tierLevel = 999
			}
		} else {
			tierLevel = 999
		}

		for _, elem := range elements {
			tierMap[elem.Element] = tierLevel
		}
	}

	return tierMap
}

// // Helper to build a recipe tree from a path
// func buildRecipeTreeFromPath(path RecipePath, element string) *RecipeNode {
// 	// Create node from root
// 	node := &RecipeNode{
// 		Element: element,
// 		Recipes: []*RecipeNode{},
// 	}

// 	// If starting element, return node
// 	if startingElements[element] {
// 		return node
// 	}

// 	// Get recipe
// 	recipe, found := path.Recipes[element]

// 	// If recipe not found
// 	if !found || len(recipe) == 0 {
// 		return node
// 	}

// 	// Create child nodes
// 	for _, ingredient := range recipe {
// 		childNode := buildRecipeTreeFromPath(path, ingredient)
// 		node.Recipes = append(node.Recipes, childNode)
// 	}

// 	return node
// }
// Helper to build a recipe tree from a path with cycle detection
func buildRecipeTreeFromPath(path RecipePath, element string) *RecipeNode {
    // Use a visited map to prevent infinite recursion
    visited := make(map[string]bool)
    return buildRecipeTreeFromPathSafe(path, element, visited, 0, 20) // Max depth of 20
}

// Safe version with cycle detection and depth limiting
func buildRecipeTreeFromPathSafe(path RecipePath, element string, visited map[string]bool, depth int, maxDepth int) *RecipeNode {
    // Check for excessive depth
    if depth > maxDepth {
        return &RecipeNode{
            Element: element,
            Recipes: []*RecipeNode{},
        }
    }
    
    // Check if we've already visited this element (cycle detection)
    if visited[element] {
        return &RecipeNode{
            Element: element,
            Recipes: []*RecipeNode{},
        }
    }
    
    // Mark as visited
    visited[element] = true
    defer func() {
        // Unmark when exiting (backtrack)
        delete(visited, element)
    }()
    
    // Create node
    node := &RecipeNode{
        Element: element,
        Recipes: []*RecipeNode{},
    }

    // If starting element, return node
    if startingElements[element] {
        return node
    }

    // Get recipe
    recipe, found := path.Recipes[element]

    // If recipe not found
    if !found || len(recipe) == 0 {
        return node
    }

    // Create child nodes
    for _, ingredient := range recipe {
        childNode := buildRecipeTreeFromPathSafe(path, ingredient, visited, depth+1, maxDepth)
        node.Recipes = append(node.Recipes, childNode)
    }

    return node
}

// Helper to check if a string is in a slice
func containsString(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}

// Helper to copy a recipe path
func copyRecipePath(path RecipePath) RecipePath {
	newPath := RecipePath{
		Elements: make([]string, len(path.Elements)),
		Recipes:  make(map[string][]string),
	}

	copy(newPath.Elements, path.Elements)

	for k, v := range path.Recipes {
		newIngredients := make([]string, len(v))
		copy(newIngredients, v)
		newPath.Recipes[k] = newIngredients
	}

	return newPath
}

// BFSSingleWithUpdates performs BFS search for a single recipe with WebSocket updates
// UPDATE BFSSingleWithUpdates - Use global counters instead of local
func BFSSingleWithUpdates(targetElement string, recipeMap map[string][][]string, tierMap map[string]int, conn *websocket.Conn) interface{} {
	// Create a WebSocket client
	client := &WebSocketClient{conn: conn}
	
	// Start timing
	startTime := time.Now()
	
	// Reset global counters at the start
	atomic.StoreInt64(&totalNodeCounted, 0)
	atomic.StoreInt64(&totalNodeVisited, 0)
	
	// Stats for progress updates - use globals instead of local
	stepCount := 0
	
	// Check if target is starting element
	if startingElements[targetElement] {
		elapsed := time.Since(startTime)
		node := &RecipeNode{
			Element: targetElement,
			Recipes: []*RecipeNode{},
		}
		
		// Send final result
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path:     node,
			Complete: true,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     1,
				StepCount:     1,
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return node
	}

	// Initialize path
	currentPath := RecipePath{
		Elements: []string{targetElement},
		Recipes:  make(map[string][]string),
	}

	// Initialize visited map 
	visited := make(map[string]bool)
	visited[targetElement] = true

	// Get valid recipes for target (this will increment totalNodeCounted)
	validRecipes := getValidRecipes(recipeMap, tierMap, targetElement)
	if len(validRecipes) == 0 {
		// No recipe found
		elapsed := time.Since(startTime)
		node := &RecipeNode{
			Element: targetElement,
			Recipes: []*RecipeNode{},
		}
		
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path:     node,
			Complete: true,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
				StepCount:     stepCount,
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return node
	}

	// Sort recipes by tier
	sort.Slice(validRecipes, func(i, j int) bool {
		return validRecipes[i].TotalTier < validRecipes[j].TotalTier
	})

	// Choose best recipe
	bestRecipe := validRecipes[0].Recipe
	currentPath.Recipes[targetElement] = bestRecipe
	stepCount++

	// Queue for BFS
	queue := []ElementToProcess{}

	// Add ingredients to queue 
	for _, ing := range bestRecipe {
		if !containsString(currentPath.Elements, ing) {
			currentPath.Elements = append(currentPath.Elements, ing)
		}

		// If not a starting element, add to queue
		if !startingElements[ing] {
			queue = append(queue, ElementToProcess{
				Element: ing,
				Depth:   1,
			})
		}
	}

	// Send first progress update
	elapsed := time.Since(startTime)
	partialTree := buildRecipeTreeFromPath(currentPath, targetElement)
	client.SendUpdate(ProcessUpdate{
		Type:     "progress",
		Element:  targetElement,
		Path:     partialTree,
		Complete: false,
		Stats: struct {
			NodeCount     int           `json:"nodeCount"`
			StepCount     int           `json:"stepCount"`
			ElapsedTime   time.Duration `json:"elapsedTime"`
			ElapsedTimeMs int64         `json:"elapsedTimeMs"`
		}{
			NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
			StepCount:     stepCount,
			ElapsedTime:   elapsed,
			ElapsedTimeMs: elapsed.Milliseconds(),
		},
	})

	// Process queue
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		atomic.AddInt64(&totalNodeVisited, 1) // Count nodes being processed

		// Skip if already visited
		if visited[current.Element] {
			continue
		}
		visited[current.Element] = true

		// Send progress update
		elapsed = time.Since(startTime)
		client.SendUpdate(ProcessUpdate{
			Type:     "progress",
			Element:  current.Element,
			Path:     buildRecipeTreeFromPath(currentPath, targetElement),
			Complete: false,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
				StepCount:     stepCount,
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})

		// Check if already has recipe
		if _, hasRecipe := currentPath.Recipes[current.Element]; hasRecipe {
			// Skip if already has recipe
			recipe := currentPath.Recipes[current.Element]
			for _, ing := range recipe {
				if !startingElements[ing] && !visited[ing] {
					queue = append(queue, ElementToProcess{
						Element: ing,
						Depth:   current.Depth + 1,
					})
				}
			}
			continue
		}

		// Get valid recipes (this will count more nodes)
		validRecipes := getValidRecipes(recipeMap, tierMap, current.Element)
		if len(validRecipes) == 0 {
			// No recipe found, return failure
			elapsed = time.Since(startTime)
			node := &RecipeNode{
				Element: targetElement,
				Recipes: []*RecipeNode{},
			}
			
			client.SendUpdate(ProcessUpdate{
				Type:     "result",
				Element:  targetElement,
				Path:     node,
				Complete: true,
				Stats: struct {
					NodeCount     int           `json:"nodeCount"`
					StepCount     int           `json:"stepCount"`
					ElapsedTime   time.Duration `json:"elapsedTime"`
					ElapsedTimeMs int64         `json:"elapsedTimeMs"`
				}{
					NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
					StepCount:     stepCount,
					ElapsedTime:   elapsed,
					ElapsedTimeMs: elapsed.Milliseconds(),
				},
			})
			
			return node
		}

		// Sort recipes by tier
		sort.Slice(validRecipes, func(i, j int) bool {
			return validRecipes[i].TotalTier < validRecipes[j].TotalTier
		})

		// Choose best recipe
		bestRecipe := validRecipes[0].Recipe
		currentPath.Recipes[current.Element] = bestRecipe
		stepCount++

		// Add ingredients to queue
		for _, ing := range bestRecipe {
			if !containsString(currentPath.Elements, ing) {
				currentPath.Elements = append(currentPath.Elements, ing)
			}

			if !startingElements[ing] && !visited[ing] {
				queue = append(queue, ElementToProcess{
					Element: ing,
					Depth:   current.Depth + 1,
				})
			}
		}
	}

	// Build final tree
	result := buildRecipeTreeFromPath(currentPath, targetElement)
	elapsed = time.Since(startTime)
	
	// Send final result
	client.SendUpdate(ProcessUpdate{
		Type:     "result",
		Element:  targetElement,
		Path:     result,
		Complete: true,
		Stats: struct {
			NodeCount     int           `json:"nodeCount"`
			StepCount     int           `json:"stepCount"`
			ElapsedTime   time.Duration `json:"elapsedTime"`
			ElapsedTimeMs int64         `json:"elapsedTimeMs"`
		}{
			NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
			StepCount:     stepCount,
			ElapsedTime:   elapsed,
			ElapsedTimeMs: elapsed.Milliseconds(),
		},
	})
	
	return result
}

// BFSMultipleWithUpdates performs BFS search for multiple recipes with WebSocket updates
func BFSMultipleWithUpdates(targetElement string, recipeMap map[string][][]string, tierMap map[string]int, recipeLimit int, conn *websocket.Conn) interface{} {
	// Create a WebSocket client
	defer func() {
        if r := recover(); r != nil {
            fmt.Printf("BFSMultipleWithUpdates panic recovered: %v\n", r)
            
            // Try to send error response to client
            client := &WebSocketClient{conn: conn}
            client.SendUpdate(ProcessUpdate{
                Type:     "error",
                Element:  targetElement,
                Path:     nil,
                Complete: true,
                Stats: struct {
                    NodeCount     int           `json:"nodeCount"`
                    StepCount     int           `json:"stepCount"`
                    ElapsedTime   time.Duration `json:"elapsedTime"`
                    ElapsedTimeMs int64         `json:"elapsedTimeMs"`
                }{
                    NodeCount:     0,
                    StepCount:     0,
                    ElapsedTime:   0,
                    ElapsedTimeMs: 0,
                },
            })
        }
    }()
	client := &WebSocketClient{conn: conn}
	startTime := time.Now()
	var branchCount int32 = 0
	maxPathsToProcess := 1000
	
	// Reset global counters at the start
	atomic.StoreInt64(&totalNodeCounted, 0)
	atomic.StoreInt64(&totalNodeVisited, 0)
	
	// Stats for progress updates
	var stepCount int32
	stepCount = 1
	
	// Channel for final results
	resultChan := make(chan *RecipeNode, recipeLimit*2)
	workChan := make(chan WorkItem, 1000)
	var wg sync.WaitGroup

	pathsProcessed := atomic.Int32{}

	// Check if target is starting element
	if startingElements[targetElement] {
		elapsed := time.Since(startTime)
		node := &RecipeNode{
			Element: targetElement,
			Recipes: []*RecipeNode{},
		}
		
		// Send final result
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path: map[string]interface{}{
				"element":     targetElement,
				"recipeCount": 1,
				"recipes":     []*RecipeNode{node},
			},
			Complete: true,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     1,
				StepCount:     int(stepCount),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return []*RecipeNode{node}
	}

	// Check recipes for element
	recipes, found := recipeMap[targetElement]
	if !found || len(recipes) == 0 {
		elapsed := time.Since(startTime)
		node := &RecipeNode{
			Element: targetElement,
			Recipes: []*RecipeNode{},
		}
		
		// Send final result
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path: map[string]interface{}{
				"element":     targetElement,
				"recipeCount": 1,
				"recipes":     []*RecipeNode{node},
			},
			Complete: true,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     1,
				StepCount:     int(stepCount),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return []*RecipeNode{node}
	}

	// This will count nodes in getValidRecipes
	validRootRecipes := getValidRecipes(recipeMap, tierMap, targetElement)

	// Sort recipes by tier
	sort.Slice(validRootRecipes, func(i, j int) bool {
		return validRootRecipes[i].TotalTier < validRootRecipes[j].TotalTier
	})

	// Choose first path to visualize
	recipesToExplore := min(len(validRootRecipes), recipeLimit)
	
	// For visual progress tracking, use the first branch
	var visualPath RecipePath
	var visualQueue []ElementToProcess
	
	for i := 0; i < recipesToExplore; i++ {
		rootPath := RecipePath{
			Elements: []string{targetElement},
			Recipes: map[string][]string{
				targetElement: validRootRecipes[i].Recipe,
			},
		}

		elemQueue := []ElementToProcess{}

		// Add ingredients to queue
		for _, ing := range validRootRecipes[i].Recipe {
			if !containsString(rootPath.Elements, ing) {
				rootPath.Elements = append(rootPath.Elements, ing)
			}

			// If not a starting element, add to queue
			if !startingElements[ing] {
				elemQueue = append(elemQueue, ElementToProcess{
					Element: ing,
					Depth:   1,
				})
			}
		}
		
		// Save first path for visualization
		if i == 0 {
			visualPath = copyRecipePath(rootPath)
			visualQueue = make([]ElementToProcess, len(elemQueue))
			copy(visualQueue, elemQueue)
			
			// Send first progress update using global counters
			atomic.AddInt32(&stepCount, 1)
			elapsed := time.Since(startTime)
			partialTree := buildRecipeTreeFromPath(visualPath, targetElement)
			client.SendUpdate(ProcessUpdate{
				Type:     "progress",
				Element:  targetElement,
				Path:     partialTree,
				Complete: false,
				Stats: struct {
					NodeCount     int           `json:"nodeCount"`
					StepCount     int           `json:"stepCount"`
					ElapsedTime   time.Duration `json:"elapsedTime"`
					ElapsedTimeMs int64         `json:"elapsedTimeMs"`
				}{
					NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
					StepCount:     int(stepCount),
					ElapsedTime:   elapsed,
					ElapsedTimeMs: elapsed.Milliseconds(),
				},
			})
		}

		// Add to work channel
		workChan <- WorkItem{
			Path:  rootPath,
			Queue: elemQueue,
		}

		branchCount++
	}
	
	// Process the visualization path in a separate goroutine
	go processVisualizationPath(visualPath, visualQueue, recipeMap, tierMap, client, &stepCount, &stepCount, startTime)

	// Create workers based on CPU count
	numWorkers := 4 // Use a fixed number instead of runtime.NumCPU()

	// Context for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Collect results
	var results []*RecipeNode
	var resultsMutex sync.Mutex

	// Start result collection goroutine
	go func() {
		for {
			select {
			case result := <-resultChan:
				resultsMutex.Lock()
				results = append(results, result)
				if len(results) >= recipeLimit {
					// Cancel when we have enough results
					cancel()
				}
				resultsMutex.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			// Add panic recovery
			defer func() {
				if r := recover(); r != nil {
					fmt.Printf("Worker panic recovered: %v\n", r)
				}
			}()
	
			for {
				select {
				case workItem, ok := <-workChan:
					if !ok {
						return
					}
	
					// Check if exceeding process limit
					if pathsProcessed.Add(1) >= int32(maxPathsToProcess) {
						continue
					}
	
					// Process path with error handling
					processPathMultiple(workItem.Path, workItem.Queue, recipeMap, tierMap,
						workChan, resultChan, &branchCount, recipeLimit)
	
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Wait for workers to finish or context to be cancelled
	// go func() {
    //     defer func() {
    //         if r := recover(); r != nil {
    //             fmt.Printf("Channel closing panic: %v\n", r)
    //         }
    //     }()
        
    //     wg.Wait()
    //     close(workChan)
    //     close(resultChan)
    // }()

	// <-ctx.Done()
	done := make(chan struct{})
go func() {
    defer close(done)
    <-ctx.Done()
}()

<-done

// Close channels safely
go func() {
    wg.Wait()
    
    // Use defer with recover to catch any panic during close
    defer func() {
        if r := recover(); r != nil {
            fmt.Printf("Channel closing recovered from panic: %v\n", r)
        }
    }()
    
    // Check if channels are already closed
    select {
    case <-workChan:
        // Already closed
    default:
        close(workChan)
    }
    
    select {
    case <-resultChan:
        // Already closed
    default:
        close(resultChan)
    }
}()

	// Ensure result doesn't exceed limit
	resultsMutex.Lock()
	if len(results) > recipeLimit {
		results = results[:recipeLimit]
	}
	
	// Send final result using global counters
	elapsed := time.Since(startTime)
	client.SendUpdate(ProcessUpdate{
		Type:     "result",
		Element:  targetElement,
		Path: map[string]interface{}{
			"element":     targetElement,
			"recipeCount": len(results),
			"recipes":     results,
		},
		Complete: true,
		Stats: struct {
			NodeCount     int           `json:"nodeCount"`
			StepCount     int           `json:"stepCount"`
			ElapsedTime   time.Duration `json:"elapsedTime"`
			ElapsedTimeMs int64         `json:"elapsedTimeMs"`
		}{
			NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
			StepCount:     int(stepCount),
			ElapsedTime:   elapsed,
			ElapsedTimeMs: elapsed.Milliseconds(),
		},
	})	
	
	resultsMutex.Unlock()

	return results
}

// Process visualization path for updates
func processVisualizationPath(currentPath RecipePath, queue []ElementToProcess,
	recipeMap map[string][][]string, tierMap map[string]int,
	client *WebSocketClient, nodeCount *int32, stepCount *int32, startTime time.Time) {
	
	// Track visited elements
	visited := make(map[string]bool)
	visited[currentPath.Elements[0]] = true // Mark target as visited
	
	// Process each element in queue
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		atomic.AddInt64(&totalNodeVisited, 1)
		
		if visited[current.Element] {
			continue
		}
		visited[current.Element] = true
		
		// Send progress update
		elapsed := time.Since(startTime)
		partialTree := buildRecipeTreeFromPath(currentPath, currentPath.Elements[0])
		client.SendUpdate(ProcessUpdate{
			Type:     "progress",
			Element:  current.Element,
			Path:     partialTree,
			Complete: false,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
				StepCount:     int(*stepCount),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		// Small delay for visualization
		// time.Sleep(100 * time.Millisecond)
		
		// Check if already has recipe
		if _, hasRecipe := currentPath.Recipes[current.Element]; hasRecipe {
			recipe := currentPath.Recipes[current.Element]
			for _, ing := range recipe {
				if !startingElements[ing] && !visited[ing] {
					queue = append(queue, ElementToProcess{
						Element: ing,
						Depth:   current.Depth + 1,
					})
				}
			}
			continue
		}
		
		// Get valid recipes
		validRecipes := getValidRecipes(recipeMap, tierMap, current.Element)
		if len(validRecipes) == 0 {
			// No valid recipes, stop visualization
			return
		}
		
		// Sort recipes by tier
		sort.Slice(validRecipes, func(i, j int) bool {
			return validRecipes[i].TotalTier < validRecipes[j].TotalTier
		})
		
		// Choose best recipe
		bestRecipe := validRecipes[0].Recipe
		currentPath.Recipes[current.Element] = bestRecipe
		atomic.AddInt32(stepCount, 1)
		
		// Add ingredients to queue
		for _, ing := range bestRecipe {
			if !containsString(currentPath.Elements, ing) {
				currentPath.Elements = append(currentPath.Elements, ing)
			}
			
			if !startingElements[ing] && !visited[ing] {
				queue = append(queue, ElementToProcess{
					Element: ing,
					Depth:   current.Depth + 1,
				})
			}
		}
	}
}

// Process a single path for multiple recipe search
func processPathMultiple(currentPath RecipePath, queue []ElementToProcess,
    recipeMap map[string][][]string, tierMap map[string]int,
    workChan chan<- WorkItem, resultChan chan<- *RecipeNode, branchCount *int32, recipeLimit int) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("processPathMultiple panic recovered: %v\n", r)
			}
		}()
	// Track visited elements
	visited := make(map[string]bool)

	// If no ingredients to process, add to results
	if len(queue) == 0 {
		node := buildRecipeTreeFromPath(currentPath, currentPath.Elements[0])
		resultChan <- node
		return
	}

	completedPath := true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		atomic.AddInt64(&totalNodeVisited, 1)
		if visited[current.Element] {
			continue
		}
		visited[current.Element] = true

		// Check if already has recipe
		if _, hasRecipe := currentPath.Recipes[current.Element]; hasRecipe {
			recipe := currentPath.Recipes[current.Element]
			for _, ing := range recipe {
				if !startingElements[ing] && !visited[ing] {
					queue = append(queue, ElementToProcess{
						Element: ing,
						Depth:   current.Depth + 1,
					})
				}
			}
			continue
		}

		// Get valid recipes
		validRecipes := getValidRecipes(recipeMap, tierMap, current.Element)
		if len(validRecipes) == 0 {
			completedPath = false
			break
		}

		// Sort recipes by tier
		sort.Slice(validRecipes, func(i, j int) bool {
			return validRecipes[i].TotalTier < validRecipes[j].TotalTier
		})

		// Choose best recipe for main branch
		bestRecipe := validRecipes[0].Recipe
		currentPath.Recipes[current.Element] = bestRecipe

		// Create alternate branches (limited)
		maxBranchesPerElement := 3
		branchesToCreate := min(len(validRecipes)-1, maxBranchesPerElement)

		for i := 1; i <= branchesToCreate && atomic.LoadInt32((*int32)(unsafe.Pointer(branchCount))) < int32(recipeLimit*3); i++ {
			// Copy current path
			branchPath := copyRecipePath(currentPath)

			// Use alternative recipe
			altRecipe := validRecipes[i].Recipe
			branchPath.Recipes[current.Element] = altRecipe

			// Create new queue
			branchQueue := []ElementToProcess{}

			// Copy visited elements
			branchVisited := make(map[string]bool)
			for k, v := range visited {
				branchVisited[k] = v
			}

			// Add alternative ingredients to queue
			for _, ing := range altRecipe {
				if !containsString(branchPath.Elements, ing) {
					branchPath.Elements = append(branchPath.Elements, ing)
				}

				// If not starting element, add to queue
				if !startingElements[ing] && !branchVisited[ing] {
					branchQueue = append(branchQueue, ElementToProcess{
						Element: ing,
						Depth:   current.Depth + 1,
					})
				}
			}

			// Check all previous elements for missing recipes
			for _, elem := range branchPath.Elements {
				if !startingElements[elem] && !branchVisited[elem] {
					if _, hasRecipe := branchPath.Recipes[elem]; !hasRecipe {
						branchQueue = append(branchQueue, ElementToProcess{
							Element: elem,
							Depth:   current.Depth,
						})
					}
				}
			}

			// Increment branch count
			atomic.AddInt32(branchCount, 1)

			// Send branch as new work
			select {
			case workChan <- WorkItem{
				Path:  branchPath,
				Queue: branchQueue,
			}:
			default:
				// Channel full, skip
			}
		}

		// Continue with main branch
		for _, ing := range bestRecipe {
			if !containsString(currentPath.Elements, ing) {
				currentPath.Elements = append(currentPath.Elements, ing)
			}

			if !startingElements[ing] && !visited[ing] {
				queue = append(queue, ElementToProcess{
					Element: ing,
					Depth:   current.Depth + 1,
				})
			}
		}
	}

	// Add completed path to results
	if completedPath {
		node := buildRecipeTreeFromPath(currentPath, currentPath.Elements[0])
		select {
		case resultChan <- node:
			// Successfully sent
		case <-time.After(100 * time.Millisecond):
			// Channel might be full or closed, skip silently
			return
		}
	}
}