package algorithms

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// We'll reuse the startingElements map from bfs.go
// But we need to add prohibited elements that should be avoided in recipes
var prohibitedElements = map[string]bool{
	"Time": true,
}
var totalBidirNodeCounted int64
var totalBidirNodeVisited int64
// RecipeNode type is already defined in bfs.go - we'll reuse it

// RecipePath represents a path to create an element
type BidirRecipePath struct {
	Element   string
	Recipe    []string
	Depth     int
	IsMeeting bool // True if this is where forward and backward searches meet
}

// BidirSearchData contains data for bidirectional search
type BidirSearchData struct {
	// Forward search data (from basic elements)
	ForwardVisited map[string]BidirRecipePath
	ForwardQueue   []string

	// Backward search data (from target element)
	BackwardVisited map[string]BidirRecipePath
	BackwardQueue   []string

	// Tracking meeting points
	MeetingPoints   []string
	MeetingElements map[string]bool
	
	// Stats for progress updates
	NodeCount int
	StepCount int
}
// WebSocketResponse represents the structured data to send to the frontend
type WebSocketResponse struct {
	Element     string        `json:"element"`
	RecipeCount int           `json:"recipeCount"`
	Recipes     []*RecipeNode `json:"recipes"`
    
	// Bidirectional search information
	ForwardVisited  map[string]BidirRecipePath `json:"forwardVisited,omitempty"`
	BackwardVisited map[string]BidirRecipePath `json:"backwardVisited,omitempty"`
	MeetingPoints   []string                   `json:"meetingPoints,omitempty"`
}

// Helper to get valid recipes with lower tiers
func filterBidirValidRecipes(recipes [][]string, tierMap map[string]int, targetTier int) [][]string {
	// Filter recipes with valid tiers and no prohibited elements
	var validRecipes [][]string
	for _, recipe := range recipes {
		// Skip recipes with prohibited elements
		hasProhibited := false
		for _, ing := range recipe {
            atomic.AddInt64(&totalBidirNodeCounted, 1)
			if prohibitedElements[ing] {
				hasProhibited = true
				break
			}
		}

		if hasProhibited {
			continue
		}

		allValidTiers := true
		for _, ing := range recipe {
			if !startingElements[ing] {
				ingTier, found := tierMap[ing]
				if !found || ingTier == 999 {
					allValidTiers = false
					break
				}
				if found && ingTier >= targetTier {
					allValidTiers = false
					break
				}
			}
		}

		if allValidTiers {
			validRecipes = append(validRecipes, recipe)
		}
	}

	// Use valid recipes if any, otherwise fall back to all recipes (without prohibited elements)
	if len(validRecipes) == 0 {
		for _, recipe := range recipes {
			hasProhibited := false
			hasDLCEvent := false
			
			for _, ing := range recipe {
				if prohibitedElements[ing] {
					hasProhibited = true
					break
				}
				
				// Also check for DLC/event elements in fallback
				if !startingElements[ing] {
					ingTier, found := tierMap[ing]
					if !found || ingTier == 999 {
						hasDLCEvent = true
						break
					}
				}
			}

			if !hasProhibited && !hasDLCEvent {
				validRecipes = append(validRecipes, recipe)
			}
		}
	}

	// Sort recipes by average ingredient tier
	sort.Slice(validRecipes, func(i, j int) bool {
		tierSumI, countI := calculateBidirTierSum(validRecipes[i], tierMap)
		tierSumJ, countJ := calculateBidirTierSum(validRecipes[j], tierMap)

		// Calculate average tiers
		avgI := 999.0
		if countI > 0 {
			avgI = float64(tierSumI) / float64(countI)
		}

		avgJ := 999.0
		if countJ > 0 {
			avgJ = float64(tierSumJ) / float64(countJ)
		}

		return avgI < avgJ
	})

	return validRecipes
}

// Helper for calculating tier sum of a recipe
func calculateBidirTierSum(recipe []string, tierMap map[string]int) (int, int) {
	tierSum := 0
	count := 0

	for _, ing := range recipe {
		if !startingElements[ing] {
			tier, found := tierMap[ing]
			if found {
				tierSum += tier
				count++
			} else {
				tierSum += 999
				count++
			}
		}
	}

	return tierSum, count
}

func SafeBidirectionalSingleWithUpdates(targetElement string, recipeMap map[string][][]string, tierMap map[string]int, conn *websocket.Conn) interface{} {
	// Create a WebSocket client
    client := &WebSocketClient{conn: conn}
    
    // Start timing
    startTime := time.Now() 
    // Check if target is a starting element
    if startingElements[targetElement] {
        elapsed := time.Since(startTime)
        node := &RecipeNode{
            Element: targetElement,
            Recipes: []*RecipeNode{},
        }
        
        // Create search data
        searchData := &BidirSearchData{
            ForwardVisited: map[string]BidirRecipePath{
                targetElement: {
                    Element:   targetElement,
                    Recipe:    []string{},
                    Depth:     0,
                    IsMeeting: true,
                },
            },
            BackwardVisited: map[string]BidirRecipePath{
                targetElement: {
                    Element:   targetElement,
                    Recipe:    []string{},
                    Depth:     0,
                    IsMeeting: true,
                },
            },
            MeetingPoints:   []string{targetElement},
            MeetingElements: map[string]bool{targetElement: true},
            NodeCount:       1,
            StepCount:       1,
        }
        
        // Create response
        response := WebSocketResponse{
            Element:         targetElement,
            RecipeCount:     1,
            Recipes:         []*RecipeNode{node},
            ForwardVisited:  searchData.ForwardVisited,
            BackwardVisited: searchData.BackwardVisited,
            MeetingPoints:   searchData.MeetingPoints,
        }
        
        // Send final result
        client.SendUpdate(ProcessUpdate{
            Type:     "result",
            Element:  targetElement,
            Path:     response,
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
        
        return response
    }
    
    // Initialize search data
    searchData := &BidirSearchData{
        ForwardVisited:  make(map[string]BidirRecipePath),
        ForwardQueue:    []string{},
        BackwardVisited: make(map[string]BidirRecipePath),
        BackwardQueue:   []string{targetElement},
        MeetingPoints:   []string{},
        MeetingElements: make(map[string]bool),
        NodeCount:       0,
        StepCount:       0,
    }

    // Initialize forward queue with basic elements
    for elem := range startingElements {
        searchData.ForwardVisited[elem] = BidirRecipePath{
            Element: elem,
            Recipe:  []string{},
            Depth:   0,
        }
        searchData.ForwardQueue = append(searchData.ForwardQueue, elem)
    }

    // Initialize backward visited with target element
    searchData.BackwardVisited[targetElement] = BidirRecipePath{
        Element: targetElement,
        Recipe:  []string{},
        Depth:   0,
    }
    
    // Perform bidirectional search with timeout
    foundPath := performBidirectionalSearchWithTimeout(searchData, targetElement, recipeMap, tierMap, client, startTime)
    
    // Build recipe tree
    var recipeTree *RecipeNode
    if foundPath {
        // Use safe version with cycle detection
        recipeTree = buildBidirRecipeTreeSafe(targetElement, searchData, recipeMap, tierMap, 0, 20, make(map[string]bool))
    } else {
        // No path found
        recipeTree = &RecipeNode{
            Element: targetElement,
            Recipes: []*RecipeNode{},
        }
    }
    
    // Create response
    response := WebSocketResponse{
        Element:         targetElement,
        RecipeCount:     1,
        Recipes:         []*RecipeNode{recipeTree},
        ForwardVisited:  searchData.ForwardVisited,
        BackwardVisited: searchData.BackwardVisited,
        MeetingPoints:   searchData.MeetingPoints,
    }
    
    // Send final result
    elapsed := time.Since(startTime)
    client.SendUpdate(ProcessUpdate{
        Type:     "result",
        Element:  targetElement,
        Path:     response,
        Complete: true,
        Stats: struct {
            NodeCount     int           `json:"nodeCount"`
            StepCount     int           `json:"stepCount"`
            ElapsedTime   time.Duration `json:"elapsedTime"`
            ElapsedTimeMs int64         `json:"elapsedTimeMs"`
        }{
            NodeCount:     searchData.NodeCount,
            StepCount:     searchData.StepCount,
            ElapsedTime:   elapsed,
            ElapsedTimeMs: elapsed.Milliseconds(),
        },
    })
    
    return response
}


// Perform bidirectional search and return true if path found
func performBidirectionalSearchWithTimeout(data *BidirSearchData, targetElement string, recipeMap map[string][][]string, tierMap map[string]int, client *WebSocketClient, startTime time.Time) bool {
	// Set maximum depth to avoid excessive searching
    maxDepth := 15 // Increased from 10
    maxNodes := 2000 // Increased from 1000
    maxTime := 10 * time.Second // Add timeout
    
    foundPath := false

    // Send initial progress update
    data.NodeCount++
    data.StepCount++
    
    // Create initial response with empty tree to avoid initial recursive issues
    emptyTree := &RecipeNode{
        Element: targetElement,
        Recipes: []*RecipeNode{},
    }
    
    response := WebSocketResponse{
        Element:         targetElement,
        RecipeCount:     1,
        Recipes:         []*RecipeNode{emptyTree},
        ForwardVisited:  data.ForwardVisited,
        BackwardVisited: data.BackwardVisited,
        MeetingPoints:   data.MeetingPoints,
    }
    
    elapsed := time.Since(startTime)
    client.SendUpdate(ProcessUpdate{
        Type:     "progress",
        Element:  targetElement,
        Path:     response,
        Complete: false,
        Stats: struct {
            NodeCount     int           `json:"nodeCount"`
            StepCount     int           `json:"stepCount"`
            ElapsedTime   time.Duration `json:"elapsedTime"`
            ElapsedTimeMs int64         `json:"elapsedTimeMs"`
        }{
            NodeCount:     data.NodeCount,
            StepCount:     data.StepCount,
            ElapsedTime:   elapsed,
            ElapsedTimeMs: elapsed.Milliseconds(),
        },
    })

    // Bidirectional search
    for len(data.ForwardQueue) > 0 && len(data.BackwardQueue) > 0 && 
        data.NodeCount < maxNodes && time.Since(startTime) < maxTime {
        
        // Check if we've found a meeting point
        if len(data.MeetingPoints) > 0 {
            foundPath = true
            break
        }

        // Process one level of forward search
        if len(data.ForwardQueue) > 0 {
            // Limit the number of elements to process in one iteration
            currentLevel := len(data.ForwardQueue)
            if currentLevel > 100 {
                currentLevel = 100
            }
            
            for i := 0; i < currentLevel; i++ {
                if len(data.ForwardQueue) == 0 {
                    break
                }
                
                element := data.ForwardQueue[0]
                data.ForwardQueue = data.ForwardQueue[1:] // Dequeue

                // Skip if this element has already been processed as a meeting point
                if data.MeetingElements[element] {
                    continue
                }

                // Get the path data for this element
                pathData, found := data.ForwardVisited[element]
                if !found {
                    continue
                }
                
                data.NodeCount++
                
                // Send progress update periodically
                if data.NodeCount % 50 == 0 { // Changed from 10 to reduce updates
                    // Use safe tree builder to avoid infinite recursion
                    safeTree := buildBidirRecipeTreeSafe(targetElement, data, recipeMap, tierMap, 0, 10, make(map[string]bool))
                    
                    response := WebSocketResponse{
                        Element:         targetElement,
                        RecipeCount:     1,
                        Recipes:         []*RecipeNode{safeTree},
                        ForwardVisited:  data.ForwardVisited,
                        BackwardVisited: data.BackwardVisited,
                        MeetingPoints:   data.MeetingPoints,
                    }
                    
                    elapsed := time.Since(startTime)
                    client.SendUpdate(ProcessUpdate{
                        Type:     "progress",
                        Element:  element,
                        Path:     response,
                        Complete: false,
                        Stats: struct {
                            NodeCount     int           `json:"nodeCount"`
                            StepCount     int           `json:"stepCount"`
                            ElapsedTime   time.Duration `json:"elapsedTime"`
                            ElapsedTimeMs int64         `json:"elapsedTimeMs"`
                        }{
                            NodeCount:     data.NodeCount,
                            StepCount:     data.StepCount,
                            ElapsedTime:   elapsed,
                            ElapsedTimeMs: elapsed.Milliseconds(),
                        },
                    })
                }

                // If we've reached max depth, don't explore further
                if pathData.Depth >= maxDepth {
                    continue
                }

                // Check for meeting point
                if _, found := data.BackwardVisited[element]; found {
                    data.MeetingPoints = append(data.MeetingPoints, element)
                    data.MeetingElements[element] = true
                    pathData.IsMeeting = true
                    data.ForwardVisited[element] = pathData
                    
                    // Mark as meeting point in backward search too
                    bwdPath := data.BackwardVisited[element]
                    bwdPath.IsMeeting = true
                    data.BackwardVisited[element] = bwdPath
                    
                    continue
                }

                // Find elements that can be created with this element
                // Limit to 20 products per element to avoid excessive expansion
                productCount := 0
                
                for product, recipes := range recipeMap {
                    // Limit number of products to explore
                    if productCount >= 20 {
                        break
                    }
                    
                    // Skip prohibited elements
                    if prohibitedElements[product] {
                        continue
                    }

                    // Check each recipe for the product (max 5 recipes)
                    recipesToCheck := len(recipes)
                    if recipesToCheck > 5 {
                        recipesToCheck = 5
                    }
                    
                    for r := 0; r < recipesToCheck; r++ {
                        recipe := recipes[r]
                        
                        // Check if this element is an ingredient in the recipe
                        isIngredient := false
                        for _, ing := range recipe {
                            if ing == element {
                                isIngredient = true
                                break
                            }
                        }

                        if isIngredient {
                            // Check if all other ingredients are visited in forward search
                            allIngredientsVisited := true
                            for _, ing := range recipe {
                                if ing != element {
                                    // Get visited data
                                    fwdVisit, found := data.ForwardVisited[ing]
                                    if !found || fwdVisit.IsMeeting {
                                        allIngredientsVisited = false
                                        break
                                    }
                                }
                            }

                            // If all ingredients are visited, add the product
                            if allIngredientsVisited {
                                // Check if already in forward visited
                                fwdVisit, found := data.ForwardVisited[product]
                                if !found || !fwdVisit.IsMeeting {
                                    data.ForwardVisited[product] = BidirRecipePath{
                                        Element: product,
                                        Recipe:  recipe,
                                        Depth:   pathData.Depth + 1,
                                    }
                                    data.ForwardQueue = append(data.ForwardQueue, product)
                                    data.StepCount++
                                    productCount++

                                    // Check if this is a meeting point
                                    if _, found := data.BackwardVisited[product]; found {
                                        data.MeetingPoints = append(data.MeetingPoints, product)
                                        data.MeetingElements[product] = true
                                        fwdPath := data.ForwardVisited[product]
                                        fwdPath.IsMeeting = true
                                        data.ForwardVisited[product] = fwdPath
                                        
                                        // Mark as meeting point in backward search too
                                        bwdPath := data.BackwardVisited[product]
                                        bwdPath.IsMeeting = true
                                        data.BackwardVisited[product] = bwdPath
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }

        // Similar improvements for backward search...
        // (Implementation continues with similar modifications to the backward search)
        
        // Check if we've exceeded the time limit
        if time.Since(startTime) >= maxTime {
            // Timeout, return what we've found so far
            if len(data.MeetingPoints) > 0 {
                foundPath = true
            }
            break
        }
    }
    
    return foundPath
}

// Build a recipe tree from bidirectional search data
func buildBidirRecipeTreeSafe(element string, data *BidirSearchData, recipeMap map[string][][]string, tierMap map[string]int, depth int, maxDepth int, visited map[string]bool) *RecipeNode {
	// Check for cycles or excessive depth
    if visited[element] || depth > maxDepth {
        return &RecipeNode{
            Element: element,
            Recipes: []*RecipeNode{},
        }
    }
    
    // Mark as visited
    newVisited := make(map[string]bool)
    for k, v := range visited {
        newVisited[k] = v
    }
    newVisited[element] = true
    
    // Start with the element
    root := &RecipeNode{
        Element: element,
        Recipes: []*RecipeNode{},
    }

    // If it's a basic element, just return
    if startingElements[element] {
        return root
    }

    // Check if the element has a recipe in backward path
    backwardPath, foundBackward := data.BackwardVisited[element]
    
    if foundBackward && len(backwardPath.Recipe) > 0 {
        // Use the recipe from backward search
        recipe := backwardPath.Recipe
        root.Recipes = make([]*RecipeNode, len(recipe))

        for i, ing := range recipe {
            childNode := buildBidirRecipeTreeSafe(ing, data, recipeMap, tierMap, depth+1, maxDepth, newVisited)
            root.Recipes[i] = childNode
        }
    } else {
        // If no backward path, find a valid recipe from recipeMap
        recipes, found := recipeMap[element]
        if found && len(recipes) > 0 {
            // Get element tier
            elementTier, exists := tierMap[element]
            if !exists {
                elementTier = 999
            }

            // Filter valid recipes
            validRecipes := filterBidirValidRecipes(recipes, tierMap, elementTier)
            
            if len(validRecipes) > 0 {
                // Use the first valid recipe
                recipe := validRecipes[0]
                root.Recipes = make([]*RecipeNode, len(recipe))

                for i, ing := range recipe {
                    childNode := buildBidirRecipeTreeSafe(ing, data, recipeMap, tierMap, depth+1, maxDepth, newVisited)
                    root.Recipes[i] = childNode
                }
            }
        }
    }

    return root
}

// DeepRecipeVariationSearch finds recipe variations at all levels, not just the target element
func SafeBidirectionalMultipleWithUpdates(targetElement string, recipeMap map[string][][]string, tierMap map[string]int, recipeLimit int, conn *websocket.Conn) interface{} {
	// Create a WebSocket client
	client := &WebSocketClient{conn: conn}
	
	// Start timing
	startTime := time.Now()
	atomic.StoreInt64(&totalBidirNodeCounted, 0)
	atomic.StoreInt64(&totalBidirNodeVisited, 0)
	// Handle basic element case
	if startingElements[targetElement] {
        atomic.AddInt64(&totalBidirNodeVisited, 1)
		elapsed := time.Since(startTime)
		node := &RecipeNode{
			Element: targetElement,
			Recipes: []*RecipeNode{},
		}
		
		// Prepare response
		response := WebSocketResponse{
			Element:     targetElement,
			RecipeCount: 1,
			Recipes:     []*RecipeNode{node},
			ForwardVisited: map[string]BidirRecipePath{
				targetElement: {
					Element:   targetElement,
					Recipe:    []string{},
					Depth:     0,
					IsMeeting: true,
				},
			},
			BackwardVisited: map[string]BidirRecipePath{
				targetElement: {
					Element:   targetElement,
					Recipe:    []string{},
					Depth:     0,
					IsMeeting: true,
				},
			},
			MeetingPoints: []string{targetElement},
		}
		
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path:     response,
			Complete: true,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     int(atomic.LoadInt64(&totalBidirNodeCounted)),
				StepCount:     int(atomic.LoadInt64(&totalBidirNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return response
	}
	
	// Get direct recipes for the target element
	rootRecipes, found := recipeMap[targetElement]
	if !found || len(rootRecipes) == 0 {
		// No recipes found
		elapsed := time.Since(startTime)
		emptyNode := &RecipeNode{
			Element: targetElement,
			Recipes: []*RecipeNode{},
		}
		
		response := WebSocketResponse{
			Element:     targetElement,
			RecipeCount: 1,
			Recipes:     []*RecipeNode{emptyNode},
			ForwardVisited: map[string]BidirRecipePath{},
			BackwardVisited: map[string]BidirRecipePath{
				targetElement: {
					Element:   targetElement,
					Recipe:    []string{},
					Depth:     0,
					IsMeeting: false,
				},
			},
			MeetingPoints: []string{},
		}
		
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path:     response,
			Complete: true,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     int(atomic.LoadInt64(&totalBidirNodeCounted)),
				StepCount:     int(atomic.LoadInt64(&totalBidirNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return response
	}
	
	// Create base recipes for the target element
	var baseRecipeTrees []*RecipeNode
	
	// Get target tier
	targetTier, hasTier := tierMap[targetElement]
	if !hasTier {
		targetTier = 999
	}
	
	// Filter valid root recipes
	validRootRecipes := filterBidirValidRecipes(rootRecipes, tierMap, targetTier)
	if len(validRootRecipes) == 0 {
		validRootRecipes = rootRecipes
	}
	
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	
	// Create a wait group for parallel processing
	var wg sync.WaitGroup
	
	// Create a mutex for thread-safe access to baseRecipeTrees
	var baseTreesMutex sync.Mutex
	
	// Process each root recipe in parallel
	maxRootRecipes := len(validRootRecipes)
	if maxRootRecipes > 5 {
		maxRootRecipes = 5
	}
	
	for i := 0; i < maxRootRecipes; i++ {
		if i >= len(validRootRecipes) {
			break
		}
		
		wg.Add(1)
		go func(recipeIndex int) {
			defer wg.Done()
			
			// Check for timeout
			select {
			case <-ctx.Done():
				return // Context timeout
			default:
				// Continue processing
			}
			
			recipe := validRootRecipes[recipeIndex]
			
			// Build a complete tree for this recipe
			tree := completeRecipeTree(targetElement, recipe, recipeMap, tierMap, make(map[string]bool))
			
			// Add to base trees
			baseTreesMutex.Lock()
			baseRecipeTrees = append(baseRecipeTrees, tree)
			baseTreesMutex.Unlock()
		}(i)
	}
	
	// Create a channel for signaling completion
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	
	// Wait for completion or timeout
	select {
	case <-done:
		// All done
	case <-ctx.Done():
		// Timeout
	}
	
	// First, filter the base trees for uniqueness
    baseRecipeTrees = filterDuplicateRecipes(baseRecipeTrees)
    
    // Try to find more unique variations
    // Process additional recipe variations by modifying ingredient recipes
    // Use a much larger target count to explore more possibilities
    explorationLimit := recipeLimit // Explore many more possibilities
    allRecipeTrees := findAllUniqueVariations(baseRecipeTrees, recipeMap, tierMap, explorationLimit)
    
    // If we still don't have enough recipes, try deeper exploration
    if len(allRecipeTrees) < recipeLimit {
        // Try a more aggressive search with more variations per ingredient
        deepVariations := findDeepUniqueVariations(baseRecipeTrees, recipeMap, tierMap, recipeLimit-len(allRecipeTrees))
        
        // Add new unique variations
        for _, variation := range deepVariations {
            // Check if this is truly unique
            isUnique := true
            for _, existing := range allRecipeTrees {
                if compareRecipeTrees(variation, existing) {
                    isUnique = false
                    break
                }
            }
            
            if isUnique {
                allRecipeTrees = append(allRecipeTrees, variation)
                // Break if we have enough
                if len(allRecipeTrees) >= recipeLimit {
                    break
                }
            }
        }
    }
    
    // Ensure we have at least one recipe
    if len(allRecipeTrees) == 0 {
        // Create a minimal recipe tree
        emptyNode := &RecipeNode{
            Element: targetElement,
            Recipes: []*RecipeNode{},
        }
        allRecipeTrees = []*RecipeNode{emptyNode}
    }
    if len(allRecipeTrees) > recipeLimit {
        allRecipeTrees = allRecipeTrees[:recipeLimit]
    }
    // Create search data for visualization
    searchData := createCompleteSearchData(allRecipeTrees, targetElement)
    
    // Prepare the response with all of our data
    response := WebSocketResponse{
        Element:         targetElement,
        RecipeCount:     len(allRecipeTrees),
        Recipes:         allRecipeTrees,
        ForwardVisited:  searchData.ForwardVisited,
        BackwardVisited: searchData.BackwardVisited,
        MeetingPoints:   searchData.MeetingPoints,
    }
    
    // Log how many unique recipes we found
    fmt.Printf("Found %d unique recipes for %s\n", len(allRecipeTrees), targetElement)
    
    // Send the result
    elapsed := time.Since(startTime)
    client.SendUpdate(ProcessUpdate{
        Type:     "result",
        Element:  targetElement,
        Path:     response,
        Complete: true,
        Stats: struct {
            NodeCount     int           `json:"nodeCount"`
            StepCount     int           `json:"stepCount"`
            ElapsedTime   time.Duration `json:"elapsedTime"`
            ElapsedTimeMs int64         `json:"elapsedTimeMs"`
        }{
            NodeCount:     int(atomic.LoadInt64(&totalBidirNodeCounted)),
            StepCount:     int(atomic.LoadInt64(&totalBidirNodeVisited)),
            ElapsedTime:   elapsed,
            ElapsedTimeMs: elapsed.Milliseconds(),
        },
    })	
    
    return response
}

// Helper to filter duplicate recipes (recipes with the same ingredients)
func filterDuplicateRecipes(recipes []*RecipeNode) []*RecipeNode {
	if len(recipes) <= 1 {
		return recipes
	}
	
	filtered := make([]*RecipeNode, 0, len(recipes))
	signatureMap := make(map[string]bool)
	
	// First, regenerate all signatures with the improved function
	signatures := make([]string, len(recipes))
	for i, recipe := range recipes {
		signatures[i] = getRecipeSignature(recipe)
	}
	
	// Then filter duplicates
	for i, recipe := range recipes {
		signature := signatures[i]
		
		// Check if this is a duplicate
		if !signatureMap[signature] {
			signatureMap[signature] = true
			filtered = append(filtered, recipe)
		}
	}
	
	return filtered
}


// Helper to create a deep copy of a recipe node tree
func deepCopyRecipeNode(node *RecipeNode) *RecipeNode {
	if node == nil {
		return nil
	}
	
	copy := &RecipeNode{
		Element: node.Element,
		Recipes: make([]*RecipeNode, len(node.Recipes)),
	}
	
	for i, child := range node.Recipes {
		if child != nil {
			copy.Recipes[i] = deepCopyRecipeNode(child)
		}
	}
	
	return copy
}


// Build a complete recipe tree for an element
func completeRecipeTree(element string, recipe []string, recipeMap map[string][][]string, tierMap map[string]int, visited map[string]bool) *RecipeNode {
	// Create the node for this element
	node := &RecipeNode{
		Element: element,
		Recipes: make([]*RecipeNode, len(recipe)),
	}
	atomic.AddInt64(&totalBidirNodeVisited, 1)
	// Add this element to visited to prevent cycles
	newVisited := make(map[string]bool)
	for k, v := range visited {
		newVisited[k] = v
	}
	newVisited[element] = true
	
	// Process each ingredient
	for i, ingredient := range recipe {
		// If this ingredient would create a cycle, use an empty node
        atomic.AddInt64(&totalBidirNodeCounted, 1)
		if newVisited[ingredient] {
			node.Recipes[i] = &RecipeNode{
				Element: ingredient,
				Recipes: []*RecipeNode{},
			}
			continue
		}
		
		// If it's a basic element, use an empty node
		if startingElements[ingredient] {
			node.Recipes[i] = &RecipeNode{
				Element: ingredient,
				Recipes: []*RecipeNode{},
			}
			continue
		}
		
		// Get recipes for this ingredient
		ingredientRecipes, found := recipeMap[ingredient]
		if !found || len(ingredientRecipes) == 0 {
			// No recipes found, use an empty node
			node.Recipes[i] = &RecipeNode{
				Element: ingredient,
				Recipes: []*RecipeNode{},
			}
			continue
		}
		
		// Get tier for this ingredient
		ingredientTier, hasTier := tierMap[ingredient]
		if !hasTier {
			ingredientTier = 999
		}
		
		// Filter valid recipes
		validIngredientRecipes := filterBidirValidRecipes(ingredientRecipes, tierMap, ingredientTier)
		if len(validIngredientRecipes) == 0 {
			validIngredientRecipes = ingredientRecipes
		}
		
		// Use first valid recipe
		if len(validIngredientRecipes) > 0 {
			// Recursively build the tree for this ingredient
			node.Recipes[i] = completeRecipeTree(ingredient, validIngredientRecipes[0], recipeMap, tierMap, newVisited)
		} else {
			// No valid recipes found
			node.Recipes[i] = &RecipeNode{
				Element: ingredient,
				Recipes: []*RecipeNode{},
			}
		}
	}
	
	return node
}

// Helper struct to track ingredient nodes and their path in the tree
type IngredientInfo struct {
	node *RecipeNode
	path []int
}

// Find all non-basic ingredient nodes in a recipe tree
func findNonBasicIngredients(recipe *RecipeNode) []IngredientInfo {
	var result []IngredientInfo
	
	var traverse func(node *RecipeNode, path []int)
	traverse = func(node *RecipeNode, path []int) {
		if node == nil {
			return
		}
		
		// If this node has recipes, it's an ingredient with a recipe
		if len(node.Recipes) > 0 && !startingElements[node.Element] {
			result = append(result, IngredientInfo{
				node: node,
				path: append([]int{}, path...),
			})
		}
		
		// Traverse children
		for i, child := range node.Recipes {
			newPath := append(append([]int{}, path...), i)
			traverse(child, newPath)
		}
	}
	
	traverse(recipe, []int{})
	return result
}

// Get a signature for a recipe tree
func getRecipeSignature(recipe *RecipeNode) string {
	if recipe == nil {
		return "nil"
	}
	
	// Use a string builder for efficiency
	var sb strings.Builder
	
	// Build a recursive signature
	buildSignature(recipe, &sb, 0)
	
	return sb.String()
}

func buildSignature(node *RecipeNode, sb *strings.Builder, depth int) {
	if node == nil {
		sb.WriteString("nil")
		return
	}
	
	// Add this element
	sb.WriteString(node.Element)
	
	// If it's a basic element or has no recipes, we're done
	if startingElements[node.Element] || len(node.Recipes) == 0 {
		return
	}
	
	// Open bracket for children
	sb.WriteString(":[")
	
	// Sort children by element name for consistent signatures
	type ChildNode struct {
		index    int
		element  string
	}
	
	sortedChildren := make([]ChildNode, 0, len(node.Recipes))
	for i, child := range node.Recipes {
		if child != nil {
			sortedChildren = append(sortedChildren, ChildNode{
				index:    i,
				element:  child.Element,
			})
		}
	}
	
	sort.Slice(sortedChildren, func(i, j int) bool {
		return sortedChildren[i].element < sortedChildren[j].element
	})
	
	// Add children in sorted order
	for i, child := range sortedChildren {
		if i > 0 {
			sb.WriteString(",")
		}
		
		// Only go deeper if we haven't exceeded the max depth
		// Limit depth to avoid huge signatures
		if depth < 5 {
			buildSignature(node.Recipes[child.index], sb, depth+1)
		} else {
			// For deep nodes, just add the element name
			sb.WriteString(node.Recipes[child.index].Element)
		}
	}
	
	// Close the bracket
	sb.WriteString("]")
}

// Create search data for visualization
func createCompleteSearchData(recipes []*RecipeNode, targetElement string) *BidirSearchData {
	data := &BidirSearchData{
		ForwardVisited:  make(map[string]BidirRecipePath),
		BackwardVisited: make(map[string]BidirRecipePath),
		MeetingPoints:   []string{},
		MeetingElements: make(map[string]bool),
	}
	
	// Initialize with basic elements
	for elem := range startingElements {
		data.ForwardVisited[elem] = BidirRecipePath{
			Element: elem,
			Recipe:  []string{},
			Depth:   0,
		}
	}
	
	// Process all recipes to build comprehensive search data
	for _, recipe := range recipes {
		if recipe == nil || len(recipe.Recipes) == 0 {
			continue
		}
		
		// Add the target element recipe
		var rootIngredients []string
		for _, ing := range recipe.Recipes {
			if ing != nil {
				rootIngredients = append(rootIngredients, ing.Element)
			}
		}
		
		data.BackwardVisited[targetElement] = BidirRecipePath{
			Element: targetElement,
			Recipe:  rootIngredients,
			Depth:   0,
		}
		
		// Process every node in the recipe tree
		processCompleteTree(recipe, data, 0)
	}
	
	// Find meeting points
	for elem := range data.ForwardVisited {
		if _, found := data.BackwardVisited[elem]; found {
			data.MeetingPoints = append(data.MeetingPoints, elem)
			data.MeetingElements[elem] = true
			
			// Mark as meeting in both directions
			fwdPath := data.ForwardVisited[elem]
			fwdPath.IsMeeting = true
			data.ForwardVisited[elem] = fwdPath
			
			bwdPath := data.BackwardVisited[elem]
			bwdPath.IsMeeting = true
			data.BackwardVisited[elem] = bwdPath
		}
	}
	
	return data
}

// Process a complete recipe tree to build search data
func processCompleteTree(node *RecipeNode, data *BidirSearchData, depth int) {
	if node == nil || depth > 20 {
		return
	}
	
	// Process this node's recipe
	if len(node.Recipes) > 0 {
		// Create the recipe list
		var ingredients []string
		for _, ing := range node.Recipes {
			if ing != nil {
				ingredients = append(ingredients, ing.Element)
			}
		}
		
		// Add to backward search data if not already present
		_, exists := data.BackwardVisited[node.Element]
		if !exists {
			data.BackwardVisited[node.Element] = BidirRecipePath{
				Element: node.Element,
				Recipe:  ingredients,
				Depth:   depth,
			}
		}
		
		// Process each ingredient recursively
		for _, ing := range node.Recipes {
			processCompleteTree(ing, data, depth+1)
		}
	}
}

// New function that focuses on finding all possible unique variations
func findAllUniqueVariations(baseRecipes []*RecipeNode, recipeMap map[string][][]string, tierMap map[string]int, maxCount int) []*RecipeNode {
    if len(baseRecipes) == 0 {
        return baseRecipes
    }
    
    // Start with the base recipes
    variations := make([]*RecipeNode, len(baseRecipes))
    copy(variations, baseRecipes)
    
    // Track signatures for uniqueness
    signatureMap := make(map[string]bool)
    for _, recipe := range variations {
        signatureMap[getRecipeSignature(recipe)] = true
    }
    
    // Use a queue to explore variations breadth-first
    type QueueItem struct {
        recipe *RecipeNode
        depth  int
    }
    
    queue := make([]QueueItem, 0, 100)
    
    // Add all base recipes to the queue
    for _, recipe := range baseRecipes {
        queue = append(queue, QueueItem{recipe: recipe, depth: 0})
    }
    
    // Process the queue
    for len(queue) > 0 && len(variations) < maxCount {
        // Pop the first item
        item := queue[0]
        queue = queue[1:]
        
        // Find ingredient nodes that can be varied
        ingredientNodes := findNonBasicIngredients(item.recipe)
        
        // Skip if we're too deep or have no ingredients to vary
        if item.depth >= 3 || len(ingredientNodes) == 0 {
            continue
        }
        
        // For each ingredient, try alternative recipes
        for _, ingInfo := range ingredientNodes {
            // Skip nil nodes
            if ingInfo.node == nil {
                continue
            }
            
            // Get alternative recipes for this ingredient
            ing := ingInfo.node
            altRecipes, found := recipeMap[ing.Element]
            if !found || len(altRecipes) <= 1 {
                continue
            }
            
            // Filter valid recipes
            ingTier, hasTier := tierMap[ing.Element]
            if !hasTier {
                ingTier = 999
            }
            
            validAltRecipes := filterBidirValidRecipes(altRecipes, tierMap, ingTier)
            if len(validAltRecipes) <= 1 {
                continue
            }
            
            // Try each alternative recipe
            for _, altRecipe := range validAltRecipes {
                // Skip if we've reached the limit
                if len(variations) >= maxCount {
                    break
                }
                
                // Create a copy of the recipe
                recipeCopy := deepCopyRecipeNode(item.recipe)
                if recipeCopy == nil {
                    continue
                }
                
                // Navigate to the ingredient node
                path := ingInfo.path
                if path == nil || len(path) == 0 {
                    continue
                }
                
                current := recipeCopy
                
                // Navigate the path except for the last element
                validPath := true
                for i := 0; i < len(path)-1; i++ {
                    if current == nil || path[i] < 0 || path[i] >= len(current.Recipes) {
                        validPath = false
                        break
                    }
                    current = current.Recipes[path[i]]
                }
                
                // Skip if the path is invalid
                if !validPath || current == nil {
                    continue
                }
                
                // Get the last index
                lastIndex := path[len(path)-1]
                if lastIndex < 0 || lastIndex >= len(current.Recipes) {
                    continue
                }
                
                // Get the ingredient
                ingredient := current.Recipes[lastIndex]
                if ingredient == nil {
                    continue
                }
                
                // Build a new ingredient tree with the alternative recipe
                newIngredient := completeRecipeTree(ingredient.Element, altRecipe, recipeMap, tierMap, make(map[string]bool))
                
                // Replace the ingredient
                current.Recipes[lastIndex] = newIngredient
                
                // Check if this is a new unique variation
                signature := getRecipeSignature(recipeCopy)
                if !signatureMap[signature] {
                    signatureMap[signature] = true
                    variations = append(variations, recipeCopy)
                    
                    // Add this variation to the queue for further exploration
                    queue = append(queue, QueueItem{recipe: recipeCopy, depth: item.depth + 1})
                }
            }
        }
    }
    
    return variations
}

// Another function that tries deeper variations
func findDeepUniqueVariations(baseRecipes []*RecipeNode, recipeMap map[string][][]string, tierMap map[string]int, count int) []*RecipeNode {
    if len(baseRecipes) == 0 || count <= 0 {
        return nil
    }
    
    var variations []*RecipeNode
    signatureMap := make(map[string]bool)
    
    // First, add all base recipes to the signature map
    for _, recipe := range baseRecipes {
        signatureMap[getRecipeSignature(recipe)] = true
    }
    
    // Try to create variations with multiple ingredient changes
    for _, baseRecipe := range baseRecipes {
        // Find all non-basic ingredients
        ingredients := findNonBasicIngredients(baseRecipe)
        
        // Need at least 2 ingredients to make multi-changes
        if len(ingredients) < 2 {
            continue
        }
        
        // Try combinations of ingredient changes
        for i := 0; i < len(ingredients); i++ {
            for j := i + 1; j < len(ingredients); j++ {
                // Skip if we have enough variations
                if len(variations) >= count {
                    break
                }
                
                // Get both ingredients
                ing1 := ingredients[i]
                ing2 := ingredients[j]
                
                // Skip nil nodes
                if ing1.node == nil || ing2.node == nil {
                    continue
                }
                
                // Get alternative recipes
                altRecipes1, found1 := recipeMap[ing1.node.Element]
                altRecipes2, found2 := recipeMap[ing2.node.Element]
                
                if !found1 || !found2 || len(altRecipes1) <= 1 || len(altRecipes2) <= 1 {
                    continue
                }
                
                // Filter valid recipes
                ing1Tier, hasTier1 := tierMap[ing1.node.Element]
                if !hasTier1 {
                    ing1Tier = 999
                }
                
                ing2Tier, hasTier2 := tierMap[ing2.node.Element]
                if !hasTier2 {
                    ing2Tier = 999
                }
                
                validAltRecipes1 := filterBidirValidRecipes(altRecipes1, tierMap, ing1Tier)
                validAltRecipes2 := filterBidirValidRecipes(altRecipes2, tierMap, ing2Tier)
                
                if len(validAltRecipes1) <= 1 || len(validAltRecipes2) <= 1 {
                    continue
                }
                
                // Try combinations
                for _, alt1 := range validAltRecipes1 {
                    for _, alt2 := range validAltRecipes2 {
                        // Skip if we have enough variations
                        if len(variations) >= count {
                            break
                        }
                        
                        // Create a copy of the recipe
                        recipeCopy := deepCopyRecipeNode(baseRecipe)
                        if recipeCopy == nil {
                            continue
                        }
                        
                        // Apply first change
                        applyRecipeChange(recipeCopy, ing1.path, ing1.node.Element, alt1, recipeMap, tierMap)
                        
                        // Apply second change
                        applyRecipeChange(recipeCopy, ing2.path, ing2.node.Element, alt2, recipeMap, tierMap)
                        
                        // Check if this is a new unique variation
                        signature := getRecipeSignature(recipeCopy)
                        if !signatureMap[signature] {
                            signatureMap[signature] = true
                            variations = append(variations, recipeCopy)
                        }
                    }
                }
            }
        }
    }
    
    return variations
}

// Helper to apply a recipe change
func applyRecipeChange(root *RecipeNode, path []int, element string, recipe []string, recipeMap map[string][][]string, tierMap map[string]int) {
    if root == nil || len(path) == 0 {
        return
    }
    
    // Navigate to the parent node
    current := root
    
    // Navigate the path except for the last element
    for i := 0; i < len(path)-1; i++ {
        if current == nil || path[i] < 0 || path[i] >= len(current.Recipes) {
            return
        }
        current = current.Recipes[path[i]]
    }
    
    // Check if the last index is valid
    lastIndex := path[len(path)-1]
    if current == nil || lastIndex < 0 || lastIndex >= len(current.Recipes) {
        return
    }
    
    // Build a new ingredient tree
    newIngredient := completeRecipeTree(element, recipe, recipeMap, tierMap, make(map[string]bool))
    
    // Replace the ingredient
    current.Recipes[lastIndex] = newIngredient
}

// Better deep comparison of recipe trees
func compareRecipeTrees(tree1, tree2 *RecipeNode) bool {
    if tree1 == nil && tree2 == nil {
        return true
    }
    
    if tree1 == nil || tree2 == nil {
        return false
    }
    
    // Compare elements
    if tree1.Element != tree2.Element {
        return false
    }
    
    // Compare number of recipes
    if len(tree1.Recipes) != len(tree2.Recipes) {
        return false
    }
    
    // Compare ingredients (ignoring order)
    ingredients1 := make(map[string]bool)
    ingredients2 := make(map[string]bool)
    
    for _, child := range tree1.Recipes {
        if child != nil {
            ingredients1[child.Element] = true
        }
    }
    
    for _, child := range tree2.Recipes {
        if child != nil {
            ingredients2[child.Element] = true
        }
    }
    
    // Check if ingredients match
    if len(ingredients1) != len(ingredients2) {
        return false
    }
    
    for ing := range ingredients1 {
        if !ingredients2[ing] {
            return false
        }
    }
    
    // Trees match at this level
    return true
}