package algorithms

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

/* Structs for JSON parsing */
type Element struct {
	Element string   `json:"element"`
	Recipes []string `json:"recipes"`
}

type ElementData map[string][]Element

/* Global variables */
var basicElements = map[string]bool{
	"Fire":  true,
	"Water": true,
	"Air":   true,
	"Earth": true,
}


var elementTiers map[string]int
var nodesVisited int64

/* Preprocessing functions */
func buildRecipeMap(data ElementData) map[string][][]string {
	recipeMap := make(map[string][][]string)
	for _, elements := range data {
		for _, elem := range elements {
			for _, rec := range elem.Recipes {
				parts := strings.Split(rec, " + ")
				if len(parts) >= 2 {
					hasProhibited := false
					for _, part := range parts {
						if prohibitedElements[part] {
							hasProhibited = true
							break
						}
					}
					if !hasProhibited {
						recipeMap[elem.Element] = append(recipeMap[elem.Element], parts)
					}
				}
			}
		}
	}
	return recipeMap
}

func calculateElementTiers(recipeMap map[string][][]string) map[string]int {
	tiers := make(map[string]int)
	for elem := range basicElements {
		tiers[elem] = 0
	}
	for element, recipes := range recipeMap {
		for _, recipe := range recipes {
			allBasic := true
			for _, ing := range recipe {
				if !basicElements[ing] {
					allBasic = false
					break
				}
			}
			if allBasic {
				tiers[element] = 1
				break
			}
		}
	}
	changed := true
	maxIter := 100
	iter := 0
	for changed && iter < maxIter {
		changed = false
		iter++
		for element := range recipeMap {
			if _, found := tiers[element]; found {
				continue
			}
			lowestRecipeTier := 9999
			for _, recipe := range recipeMap[element] {
				allIngsHaveTiers := true
				maxIngTier := 0
				for _, ing := range recipe {
					t, found := tiers[ing]
					if !found {
						allIngsHaveTiers = false
						break
					}
					if t > maxIngTier {
						maxIngTier = t
					}
				}
				if allIngsHaveTiers && maxIngTier+1 < lowestRecipeTier {
					lowestRecipeTier = maxIngTier + 1
				}
			}
			if lowestRecipeTier < 9999 {
				tiers[element] = lowestRecipeTier
				changed = true
			}
		}
	}
	return tiers
}

/* Utility Helpers */
func findBestRecipe(element string, recipeMap map[string][][]string, maxAllowedTier int) []string {
	recipes, found := recipeMap[element]
	if !found || len(recipes) == 0 {
		return nil
	}
	elementTier, found := elementTiers[element]
	if !found {
		elementTier = 999
	}
	var validRecipes [][]string
	for _, recipe := range recipes {
		hasProhibited := false
		for _, ing := range recipe {
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
			atomic.AddInt64(&totalNodeCounted, 1)
			if !basicElements[ing] {
				ingTier, found := elementTiers[ing]
				if found && ingTier >= elementTier {
					allValidTiers = false
					break
				}
			}
		}
		if allValidTiers {
			validRecipes = append(validRecipes, recipe)
		}
	}
	recipesToUse := validRecipes
	if len(validRecipes) == 0 {
		for _, recipe := range recipes {
			hasProhibited := false
			for _, ing := range recipe {
				if prohibitedElements[ing] {
					hasProhibited = true
					break
				}
			}
			if !hasProhibited {
				recipesToUse = append(recipesToUse, recipe)
			}
		}
	}
	if len(recipesToUse) == 0 {
		return nil
	}
	sort.Slice(recipesToUse, func(i, j int) bool {
		sumI, sumJ := 0, 0
		for _, ing := range recipesToUse[i] {
			if t, ok := elementTiers[ing]; ok {
				sumI += t
			} else {
				sumI += 999
			}
		}
		for _, ing := range recipesToUse[j] {
			if t, ok := elementTiers[ing]; ok {
				sumJ += t
			} else {
				sumJ += 999
			}
		}
		return (sumI / len(recipesToUse[i])) < (sumJ / len(recipesToUse[j]))
	})
	return recipesToUse[0]
}

func filterAndSortRecipes(recipes [][]string, targetTier int) [][]string {
	var validRecipes [][]string
	for _, recipe := range recipes {
		hasProhibited := false
		hasHigherTier := false
		for _, ing := range recipe {
			// Count each ingredient check
			atomic.AddInt64(&totalNodeCounted, 1)
			
			if prohibitedElements[ing] {
				hasProhibited = true
				break
			}
			if !basicElements[ing] {
				ingTier, found := elementTiers[ing]
				if !found {
					hasProhibited = true
					break
				}
				if ingTier >= targetTier {
					hasHigherTier = true
					break
				}
			}
		}
		if !hasProhibited && !hasHigherTier {
			validRecipes = append(validRecipes, recipe)
		}
	}
	return validRecipes
}

func generateRecipeHash(node *RecipeNode) string {
	if node == nil {
		return ""
	}
	if len(node.Recipes) == 0 {
		return node.Element
	}
	var parts []string
	for _, recipe := range node.Recipes {
		parts = append(parts, generateRecipeHash(recipe))
	}
	sort.Strings(parts)
	return fmt.Sprintf("%s:[%s]", node.Element, strings.Join(parts, "+"))
}

func countIngredients(node *RecipeNode) int {
	if node == nil {
		return 0
	}
	count := len(node.Recipes)
	for _, recipe := range node.Recipes {
		count += countIngredients(recipe)
	}
	return count
}

func isElementAllowedForTier(element string, targetTier int) bool {
	if basicElements[element] {
		return true
	}
	if prohibitedElements[element] {
		return false
	}
	elementTier, found := elementTiers[element]
	if !found {
		return false
	}
	return elementTier < targetTier
}

/* DFS-based Recipe Tree Builders with WebSocket updates */
// Builds the recipe tree for a single recipe using DFS with updates.
func buildRecipeTreeDFSWithUpdates(node *RecipeNode, recipeMap map[string][][]string, visited map[string]bool, client *WebSocketClient, targetElement string, startTime time.Time) bool {
	if visited[node.Element] {
		node.Element += " (cycle)"
		node.Recipes = []*RecipeNode{}
		return false
	}
	visited[node.Element] = true
	
	// Count nodes like in the updated buildCompleteRecipeTree
	atomic.AddInt64(&totalNodeVisited, 1)
	atomic.AddInt64(&nodesVisited, 1)
	
	// Send progress update using global counters
	if client != nil {
		elapsed := time.Since(startTime)
		client.SendUpdate(ProcessUpdate{
			Type:     "progress",
			Element:  node.Element,
			Path:     node,
			Complete: false,
			Stats: struct {
				NodeCount     int           `json:"nodeCount"`
				StepCount     int           `json:"stepCount"`
				ElapsedTime   time.Duration `json:"elapsedTime"`
				ElapsedTimeMs int64         `json:"elapsedTimeMs"`
			}{
				NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
				StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
	}
	
	if basicElements[node.Element] {
		node.Recipes = []*RecipeNode{}
		return true
	}
	elementTier, found := elementTiers[node.Element]
	if !found {
		elementTier = 999
	}
	// This will count nodes when searching for the best recipe
	bestRecipe := findBestRecipe(node.Element, recipeMap, elementTier)
	if bestRecipe == nil {
		node.Recipes = []*RecipeNode{}
		return false
	}
	node.Recipes = make([]*RecipeNode, len(bestRecipe))
	allIngredientsValid := true
	for i, ing := range bestRecipe {
		childNode := &RecipeNode{Element: ing, Recipes: []*RecipeNode{}}
		subVisited := make(map[string]bool)
		for k, v := range visited {
			subVisited[k] = v
		}
		ingValid := true
		if !basicElements[ing] {
			ingValid = buildRecipeTreeDFSWithUpdates(childNode, recipeMap, subVisited, client, targetElement, startTime)
		}
		if !ingValid {
			allIngredientsValid = false
		}
		node.Recipes[i] = childNode
	}
	return allIngredientsValid
}

// Returns a single recipe tree with WebSocket updates.
// UPDATE DFSSingleWithUpdates - Use global counters
func DFSSingleWithUpdates(targetElement string, recipeMap map[string][][]string, tierMap map[string]int, conn *websocket.Conn) interface{} {
	// Create a WebSocket client
	client := &WebSocketClient{conn: conn}
	
	// Start timing
	startTime := time.Now()
	
	// Reset global counters at the start
	atomic.StoreInt64(&totalNodeCounted, 0)
	atomic.StoreInt64(&totalNodeVisited, 0)
	
	// Set global tier map
	elementTiers = tierMap
	atomic.StoreInt64(&nodesVisited, 0)
	
	if basicElements[targetElement] {
		atomic.AddInt64(&nodesVisited, 1)
		atomic.AddInt64(&totalNodeVisited, 1)
		elapsed := time.Since(startTime)
		node := &RecipeNode{Element: targetElement, Recipes: []*RecipeNode{}}
		
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
				NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
				StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return node
	}
	
	// This will count nodes when searching for valid recipes
	recipes, found := recipeMap[targetElement]
	if !found || len(recipes) == 0 {
		elapsed := time.Since(startTime)
		node := &RecipeNode{Element: targetElement, Recipes: []*RecipeNode{}}
		
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
				StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return node
	}
	targetTier, found := elementTiers[targetElement]
	if !found {
		targetTier = 999
	}
	recipesToTry := filterAndSortRecipes(recipes, targetTier)
	
	for _, recipe := range recipesToTry {
		root := &RecipeNode{Element: targetElement, Recipes: make([]*RecipeNode, len(recipe))}
		allValid := true
		for i, ing := range recipe {
			if basicElements[ing] {
				root.Recipes[i] = &RecipeNode{Element: ing, Recipes: []*RecipeNode{}}
			} else {
				node := &RecipeNode{Element: ing, Recipes: []*RecipeNode{}}
				visited := make(map[string]bool)
				if !buildRecipeTreeDFSWithUpdates(node, recipeMap, visited, client, targetElement, startTime) {
					allValid = false
					break
				}
				root.Recipes[i] = node
			}
		}
		if allValid {
			// Send final result
			elapsed := time.Since(startTime)
			client.SendUpdate(ProcessUpdate{
				Type:     "result",
				Element:  targetElement,
				Path:     root,
				Complete: true,
				Stats: struct {
					NodeCount     int           `json:"nodeCount"`
					StepCount     int           `json:"stepCount"`
					ElapsedTime   time.Duration `json:"elapsedTime"`
					ElapsedTimeMs int64         `json:"elapsedTimeMs"`
				}{
					NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
					StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
					ElapsedTime:   elapsed,
					ElapsedTimeMs: elapsed.Milliseconds(),
				},
			})
			
			return root
		}
	}
	
	elapsed := time.Since(startTime)
	node := &RecipeNode{Element: targetElement, Recipes: []*RecipeNode{}}
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
			StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
			ElapsedTime:   elapsed,
			ElapsedTimeMs: elapsed.Milliseconds(),
		},
	})
	
	return node
}
/* Enhanced DFS for finding multiple recipes with slight differences */
// Creates multi-point variations from an existing recipe tree.
func createMultiPointVariation(baseRecipe *RecipeNode, recipeMap map[string][][]string,
	results *[]*RecipeNode, uniqueRecipes map[string]bool,
	mutex *sync.Mutex, recipeLimit int) {
	targetElement := baseRecipe.Element
	targetTier, found := elementTiers[targetElement]
	if !found {
		targetTier = 999
	}
	type ingredient struct {
		path    []int
		element string
	}
	var ingredients []ingredient
	var findIngredients func(node *RecipeNode, path []int)
	findIngredients = func(node *RecipeNode, path []int) {
		for i, ing := range node.Recipes {
			if !basicElements[ing.Element] && isElementAllowedForTier(ing.Element, targetTier) {
				newPath := append(append([]int{}, path...), i)
				ingredients = append(ingredients, ingredient{path: newPath, element: ing.Element})
				findIngredients(ing, newPath)
			}
		}
	}
	findIngredients(baseRecipe, []int{})
	if len(ingredients) >= 2 {
		ing1 := ingredients[len(ingredients)%len(ingredients)]
		ing2 := ingredients[(len(ingredients)/2)%len(ingredients)]
		recipes1, found1 := recipeMap[ing1.element]
		if !found1 || len(recipes1) == 0 {
			return
		}
		recipes2, found2 := recipeMap[ing2.element]
		if !found2 || len(recipes2) == 0 {
			return
		}
		tier1, found := elementTiers[ing1.element]
		if !found {
			tier1 = 999
		}
		tier2, found := elementTiers[ing2.element]
		if !found {
			tier2 = 999
		}
		valid1 := filterAndSortRecipes(recipes1, tier1)
		valid2 := filterAndSortRecipes(recipes2, tier2)
		if len(valid1) == 0 || len(valid2) == 0 {
			return
		}
		for i := 0; i < min(10, len(valid1)); i++ {
			for j := 0; j < min(10, len(valid2)); j++ {
				mutex.Lock()
				if len(*results) >= recipeLimit {
					mutex.Unlock()
					return
				}
				mutex.Unlock()
				newTree := deepCopyRecipeNode(baseRecipe)
				current := newTree
				for k := 0; k < len(ing1.path)-1; k++ {
					if ing1.path[k] < len(current.Recipes) {
						current = current.Recipes[ing1.path[k]]
					} else {
						current = nil
						break
					}
				}
				if current == nil {
					continue
				}
				lastIdx := ing1.path[len(ing1.path)-1]
				if lastIdx >= len(current.Recipes) {
					continue
				}
				sub1 := buildSubRecipeTree(valid1[i], ing1.element, recipeMap)
				if sub1 == nil {
					continue
				}
				current.Recipes[lastIdx] = sub1
				current = newTree
				for k := 0; k < len(ing2.path)-1; k++ {
					if ing2.path[k] < len(current.Recipes) {
						current = current.Recipes[ing2.path[k]]
					} else {
						current = nil
						break
					}
				}
				if current == nil {
					continue
				}
				lastIdx = ing2.path[len(ing2.path)-1]
				if lastIdx >= len(current.Recipes) {
					continue
				}
				sub2 := buildSubRecipeTree(valid2[j], ing2.element, recipeMap)
				if sub2 == nil {
					continue
				}
				current.Recipes[lastIdx] = sub2
				hash := generateRecipeHash(newTree)
				mutex.Lock()
				if !uniqueRecipes[hash] && len(*results) < recipeLimit {
					uniqueRecipes[hash] = true
					*results = append(*results, newTree)
				}
				mutex.Unlock()
			}
		}
	}
}

// Explores all variations starting from a given base recipe with updates.
func exploreAllRecipeVariationsWithUpdates(element string, ingredients []string, recipeMap map[string][][]string,
	targetTier int, results *[]*RecipeNode, uniqueRecipes map[string]bool,
	mutex *sync.Mutex, recipeLimit int, client *WebSocketClient, startTime time.Time) {
	root := buildInitialRecipeTree(element, ingredients, recipeMap, targetTier)
	if root == nil {
		return
	}
	rootHash := generateRecipeHash(root)
	mutex.Lock()
	if !uniqueRecipes[rootHash] && len(*results) < recipeLimit {
		uniqueRecipes[rootHash] = true
		*results = append(*results, root)
		
		// Send progress update for new recipe found
		if client != nil {
			elapsed := time.Since(startTime)
			client.SendUpdate(ProcessUpdate{
				Type:     "progress",
				Element:  element,
				Path: map[string]interface{}{
					"element":     element,
					"recipeCount": len(*results),
					"recipes":     *results,
				},
				Complete: false,
				Stats: struct {
					NodeCount     int           `json:"nodeCount"`
					StepCount     int           `json:"stepCount"`
					ElapsedTime   time.Duration `json:"elapsedTime"`
					ElapsedTimeMs int64         `json:"elapsedTimeMs"`
				}{
					NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
					StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
					ElapsedTime:   elapsed,
					ElapsedTimeMs: elapsed.Milliseconds(),
				},
			})
		}
	}
	limitReached := len(*results) >= recipeLimit
	mutex.Unlock()
	if limitReached {
		return
	}
	type explorationKey struct {
		element string
		recipe  string
		path    string
	}
	visited := make(map[explorationKey]bool)
	type workItem struct {
		node  *RecipeNode
		path  []int
		depth int
	}
	workQueue := []workItem{{node: root, path: []int{}, depth: 0}}
	maxQueueSize := 200000
	maxDepth := 300
	explorationBudget := recipeLimit * 50000
	explorationCount := 0
	for len(workQueue) > 0 && explorationCount < explorationBudget {
		explorationCount++
		item := workQueue[0]
		workQueue = workQueue[1:]
		mutex.Lock()
		limitReached = len(*results) >= recipeLimit
		mutex.Unlock()
		if limitReached {
			break
		}
		if item.depth > maxDepth {
			continue
		}
		if len(item.path) == 0 {
			var findNonBasics func(node *RecipeNode, path []int, depth int)
			findNonBasics = func(node *RecipeNode, path []int, depth int) {
				targetTier, _ := elementTiers[element]
				if !basicElements[node.Element] && len(path) > 0 && isElementAllowedForTier(node.Element, targetTier) {
					newPath := append(append([]int{}, path...), 0)
					if len(workQueue) < maxQueueSize {
						workQueue = append(workQueue, workItem{node: item.node, path: newPath, depth: item.depth})
						if len(newPath) > 2 {
							partialPath := newPath[:len(newPath)-1]
							workQueue = append(workQueue, workItem{node: item.node, path: partialPath, depth: item.depth})
						}
					}
				}
				for i, ing := range node.Recipes {
					if len(workQueue) >= maxQueueSize {
						return
					}
					newPath := append(append([]int{}, path...), i)
					if !basicElements[ing.Element] && isElementAllowedForTier(ing.Element, targetTier) {
						workQueue = append(workQueue, workItem{node: item.node, path: newPath, depth: item.depth})
						if len(ing.Recipes) > 0 && depth < 20 {
							findNonBasics(ing, newPath, depth+1)
						}
					}
				}
			}
			findNonBasics(item.node, []int{}, 0)
			continue
		}
		var currentNode *RecipeNode = item.node
		var parent *RecipeNode = nil
		var parentIndex int = -1
		for i, idx := range item.path {
			if i == len(item.path)-1 {
				parent = currentNode
				parentIndex = idx
			} else if idx < len(currentNode.Recipes) {
				currentNode = currentNode.Recipes[idx]
			} else {
				parent = nil
				break
			}
		}
		if parent == nil || parentIndex < 0 || parentIndex >= len(parent.Recipes) {
			continue
		}
		ingNode := parent.Recipes[parentIndex]
		ingElement := ingNode.Element
		ingTier, found := elementTiers[ingElement]
		if !found {
			ingTier = 999
		}
		targetTier, _ := elementTiers[element]
		if !isElementAllowedForTier(ingElement, targetTier) {
			continue
		}
		ingRecipes, found := recipeMap[ingElement]
		if !found || len(ingRecipes) == 0 {
			continue
		}
		validIngRecipes := filterAndSortRecipes(ingRecipes, ingTier)
		if len(validIngRecipes) == 0 {
			continue
		}
		pathStr := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(item.path)), ","), "[]")
		recipesToTry := make([][]string, len(validIngRecipes))
		copy(recipesToTry, validIngRecipes)
		if len(pathStr)%3 == 0 {
			for i, j := 0, len(recipesToTry)-1; i < j; i, j = i+1, j-1 {
				recipesToTry[i], recipesToTry[j] = recipesToTry[j], recipesToTry[i]
			}
		}
		for _, altRecipe := range recipesToTry {
			key := explorationKey{
				element: ingElement,
				recipe:  strings.Join(altRecipe, "+"),
				path:    pathStr,
			}
			if visited[key] {
				continue
			}
			visited[key] = true
			subTree := buildSubRecipeTree(altRecipe, ingElement, recipeMap)
			if subTree == nil {
				continue
			}
			newTree := deepCopyRecipeNode(item.node)
			currentNode = newTree
			for i, idx := range item.path {
				if i == len(item.path)-1 {
					if idx < len(currentNode.Recipes) {
						currentNode.Recipes[idx] = subTree
					}
				} else if idx < len(currentNode.Recipes) {
					currentNode = currentNode.Recipes[idx]
				} else {
					currentNode = nil
					break
				}
			}
			if currentNode == nil {
				continue
			}
			newHash := generateRecipeHash(newTree)
			mutex.Lock()
			if !uniqueRecipes[newHash] && len(*results) < recipeLimit {
				uniqueRecipes[newHash] = true
				*results = append(*results, newTree)
				
				// Send progress update for new recipe found
				if client != nil && len(*results)%5 == 0 { // Send updates every 5 recipes found
					elapsed := time.Since(startTime)
					client.SendUpdate(ProcessUpdate{
						Type:     "progress",
						Element:  element,
						Path: map[string]interface{}{
							"element":     element,
							"recipeCount": len(*results),
							"recipes":     *results,
						},
						Complete: false,
						Stats: struct {
							NodeCount     int           `json:"nodeCount"`
							StepCount     int           `json:"stepCount"`
							ElapsedTime   time.Duration `json:"elapsedTime"`
							ElapsedTimeMs int64         `json:"elapsedTimeMs"`
						}{
							NodeCount:     int(atomic.LoadInt64(&totalNodeCounted)),
							StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
							ElapsedTime:   elapsed,
							ElapsedTimeMs: elapsed.Milliseconds(),
						},
					})
				}
				
				if len(*results) >= recipeLimit {
					mutex.Unlock()
					return
				}
			}
			mutex.Unlock()
			if len(workQueue) < maxQueueSize {
				workQueue = append(workQueue, workItem{node: newTree, path: []int{}, depth: item.depth + 1})
				if item.depth < maxDepth-3 {
					workQueue = append(workQueue, workItem{node: newTree, path: item.path, depth: item.depth + 1})
				}
				if len(item.path) > 1 && item.depth < maxDepth-2 {
					partialPath := item.path[:len(item.path)/2]
					workQueue = append(workQueue, workItem{node: newTree, path: partialPath, depth: item.depth + 1})
				}
			}
		}
	}
}

// Builds the initial complete recipe tree.
func buildInitialRecipeTree(element string, ingredients []string, recipeMap map[string][][]string, targetTier int) *RecipeNode {
	root := &RecipeNode{Element: element, Recipes: make([]*RecipeNode, len(ingredients))}
	for i, ing := range ingredients {
		if basicElements[ing] {
			root.Recipes[i] = &RecipeNode{Element: ing, Recipes: []*RecipeNode{}}
		} else {
			ingTier, found := elementTiers[ing]
			if !found {
				ingTier = 999
			}
			if ingTier >= targetTier {
				return nil
			}
			subTree := buildCompleteRecipeTree(ing, recipeMap, make(map[string]bool))
			if subTree == nil {
				return nil
			}
			root.Recipes[i] = subTree
		}
	}
	return root
}

// Builds a complete recipe tree for an element.
func buildCompleteRecipeTree(element string, recipeMap map[string][][]string, visited map[string]bool) *RecipeNode {
	if visited[element] {
		return nil
	}
	visited[element] = true
	atomic.AddInt64(&totalNodeVisited, 1)
	atomic.AddInt64(&nodesVisited, 1)
	if basicElements[element] {
		return &RecipeNode{Element: element, Recipes: []*RecipeNode{}}
	}
	elementTier, found := elementTiers[element]
	if !found {
		elementTier = 999
	}
	recipes, found := recipeMap[element]
	if !found || len(recipes) == 0 {
		return nil
	}
	validRecipes := filterAndSortRecipes(recipes, elementTier)
	if len(validRecipes) == 0 {
		return nil
	}
	// Try more varied recipe selection for ingredients
	recipeIndex := 0
	if len(validRecipes) > 1 {
		// Try to take rare recipes that have uncommon ingredients
		// This works better than just taking recipes with more ingredients

		// Combine several factors to create a more varied selection
		// 1. The element name length - gives consistent but different results for different elements
		nameValue := len(element) % len(validRecipes)

		// 2. The number of visited elements - changes as exploration proceeds
		visitValue := len(visited) % len(validRecipes)

		// 3. A counter that increases with each node - adds global variation
		counterValue := int(nodesVisited) % len(validRecipes)

		// Combine these to get a varied but deterministic recipe selector
		recipeIndex = (nameValue*7 + visitValue*13 + counterValue*17) % len(validRecipes)
	}

	bestRecipe := validRecipes[recipeIndex]

	node := &RecipeNode{Element: element, Recipes: make([]*RecipeNode, len(bestRecipe))}
	for i, ing := range bestRecipe {
		if basicElements[ing] {
			node.Recipes[i] = &RecipeNode{Element: ing, Recipes: []*RecipeNode{}}
			continue
		}
		ingTier, found := elementTiers[ing]
		if !found {
			ingTier = 999
		}
		if ingTier >= elementTier {
			return nil
		}
		subVisited := make(map[string]bool)
		for k, v := range visited {
			subVisited[k] = v
		}
		subNode := buildCompleteRecipeTree(ing, recipeMap, subVisited)
		if subNode == nil {
			return nil
		}
		node.Recipes[i] = subNode
	}
	return node
}

// Build a sub-recipe tree for a specific ingredient.
func buildSubRecipeTree(recipe []string, element string, recipeMap map[string][][]string) *RecipeNode {
	node := &RecipeNode{Element: element, Recipes: make([]*RecipeNode, len(recipe))}
	elementTier, found := elementTiers[element]
	if !found {
		elementTier = 999
	}
	for i, ing := range recipe {
		atomic.AddInt64(&totalNodeCounted, 1)
		if basicElements[ing] {
			node.Recipes[i] = &RecipeNode{Element: ing, Recipes: []*RecipeNode{}}
			continue
		}
		ingTier, found := elementTiers[ing]
		if !found {
			return nil
		}
		if ingTier >= elementTier {
			return nil
		}
		subTree := buildCompleteRecipeTree(ing, recipeMap, make(map[string]bool))
		if subTree == nil {
			return nil
		}
		node.Recipes[i] = subTree
	}
	return node
}

func sortRecipesByComplexity(recipes []*RecipeNode) {
	sort.Slice(recipes, func(i, j int) bool {
		return countIngredients(recipes[i]) < countIngredients(recipes[j])
	})
}

/* Main Recipe Search Functions */
// Returns multiple recipe trees exploring slight variations with WebSocket updates.
// UPDATE DFSMultipleWithUpdates - Use global counters
func DFSMultipleWithUpdates(targetElement string, recipeMap map[string][][]string, tierMap map[string]int, recipeLimit int, conn *websocket.Conn) interface{} {
	// Create a WebSocket client
	client := &WebSocketClient{conn: conn}
	
	// Start timing
	startTime := time.Now()
	
	// Reset global counters at the start
	atomic.StoreInt64(&totalNodeCounted, 0)
	atomic.StoreInt64(&totalNodeVisited, 0)
	
	// Set global tier map
	elementTiers = tierMap
	atomic.StoreInt64(&nodesVisited, 0)
	
	var results []*RecipeNode
	if basicElements[targetElement] {
		atomic.AddInt64(&nodesVisited, 1)
		atomic.AddInt64(&totalNodeVisited, 1)
		elapsed := time.Since(startTime)
		results = []*RecipeNode{{Element: targetElement, Recipes: []*RecipeNode{}}}
		
		// Send final result
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path: map[string]interface{}{
				"element":     targetElement,
				"recipeCount": 1,
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
				StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return results
	}
	targetTier, found := elementTiers[targetElement]
	if !found {
		targetTier = 999
	}
	recipes, found := recipeMap[targetElement]
	if !found || len(recipes) == 0 {
		elapsed := time.Since(startTime)
		results = []*RecipeNode{{Element: targetElement, Recipes: []*RecipeNode{}}}
		
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path: map[string]interface{}{
				"element":     targetElement,
				"recipeCount": 1,
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
				StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return results
	}
	atomic.AddInt64(&nodesVisited, 1)
	atomic.AddInt64(&totalNodeVisited, 1)
	validRecipes := filterAndSortRecipes(recipes, targetTier)
	if len(validRecipes) == 0 {
		elapsed := time.Since(startTime)
		results = []*RecipeNode{{Element: targetElement, Recipes: []*RecipeNode{}}}
		
		client.SendUpdate(ProcessUpdate{
			Type:     "result",
			Element:  targetElement,
			Path: map[string]interface{}{
				"element":     targetElement,
				"recipeCount": 1,
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
				StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
				ElapsedTime:   elapsed,
				ElapsedTimeMs: elapsed.Milliseconds(),
			},
		})
		
		return results
	}
	uniqueRecipes := make(map[string]bool)
	var mutex sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < len(validRecipes); i++ {
		wg.Add(1)
		go func(ings []string) {
			defer wg.Done()
			mutex.Lock()
			limitReached := len(results) >= recipeLimit
			mutex.Unlock()
			if limitReached {
				return
			}
			exploreAllRecipeVariationsWithUpdates(targetElement, ings, recipeMap, targetTier, &results, uniqueRecipes, &mutex, recipeLimit, client, startTime)
		}(validRecipes[i])
	}
	wg.Wait()
	if len(results) < recipeLimit {
		for _, baseRecipe := range results {
			mutex.Lock()
			if len(results) >= recipeLimit {
				mutex.Unlock()
				break
			}
			mutex.Unlock()
			createMultiPointVariation(baseRecipe, recipeMap, &results, uniqueRecipes, &mutex, recipeLimit)
		}
	}
	sortRecipesByComplexity(results)
	
	// Send final result
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
			StepCount:     int(atomic.LoadInt64(&totalNodeVisited)),
			ElapsedTime:   elapsed,
			ElapsedTimeMs: elapsed.Milliseconds(),
		},
	})
	
	return results
}