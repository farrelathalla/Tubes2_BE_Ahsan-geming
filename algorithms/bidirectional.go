package algorithms

import (
	"fmt"
	"strings"

	"github.com/yourname/tubes2-be-ahsan-geming/scraper"
)

// RecipeMap maps a combination (sorted key) to result
type RecipeMap map[string]string

// ReverseMap maps element to its possible ingredients
type ReverseMap map[string][][]string

// Bidirectional performs bidirectional search from base to target
func Bidirectional(target string, elements []scraper.ElementData) []string {
	forwardQueue := [][]string{{}} // forward path starts empty
	backwardQueue := [][]string{{target}}

	forwardVisited := map[string][]string{}
	backwardVisited := map[string][]string{}

	recipeMap, reverseMap := BuildMaps(elements)

	// Initialize forward from base elements (no recipes)
	for _, el := range elements {
		if len(el.Recipes) == 0 {
			forwardQueue = append(forwardQueue, []string{el.Element})
			forwardVisited[el.Element] = []string{el.Element}
		}
	}

	// Initialize backward
	backwardVisited[target] = []string{target}

	for len(forwardQueue) > 0 && len(backwardQueue) > 0 {
		// Expand forward
		newFwd := [][]string{}
		for _, path := range forwardQueue {
			last := path[len(path)-1]
			for k, res := range recipeMap {
				if strings.Contains(k, last) {
					if _, ok := forwardVisited[res]; !ok {
						newPath := append([]string{}, path...)
						newPath = append(newPath, res)
						forwardVisited[res] = newPath
						newFwd = append(newFwd, newPath)

						// Check for meet point
						if bPath, ok := backwardVisited[res]; ok {
							fmt.Println("Meet at:", res)
							return append(newPath, reverse(bPath[1:])...)
						}
					}
				}
			}
		}
		forwardQueue = newFwd

		// Expand backward
		newBwd := [][]string{}
		for _, path := range backwardQueue {
			last := path[len(path)-1]
			for _, recipe := range reverseMap[last] {
				for _, ingredient := range recipe {
					if _, ok := backwardVisited[ingredient]; !ok {
						newPath := append([]string{ingredient}, path...)
						backwardVisited[ingredient] = newPath
						newBwd = append(newBwd, newPath)

						// Check for meet point
						if fPath, ok := forwardVisited[ingredient]; ok {
							fmt.Println("Meet at:", ingredient)
							return append(fPath, reverse(newPath[1:])...)
						}
					}
				}
			}
		}
		backwardQueue = newBwd
	}

	return []string{"No path found"}
}

// BuildMaps builds recipe and reverse lookup
func BuildMaps(elements []scraper.ElementData) (RecipeMap, ReverseMap) {
	recipeMap := RecipeMap{}
	reverseMap := ReverseMap{}

	for _, e := range elements {
		if len(e.Recipes) == 0 {
			continue
		}
		key := NormalizeRecipe(e.Recipes)
		recipeMap[key] = e.Element

		reverseMap[e.Element] = [][]string{}
		reverseMap[e.Element] = append(reverseMap[e.Element], e.Recipes)
	}

	return recipeMap, reverseMap
}

// NormalizeRecipe turns a list of ingredients into a sorted string key
func NormalizeRecipe(r []string) string {
	if len(r) == 1 {
		return r[0]
	}
	if r[0] < r[1] {
		return r[0] + "+" + r[1]
	}
	return r[1] + "+" + r[0]
}

// Reverse a slice
func reverse(s []string) []string {
	reversed := make([]string, len(s))
	for i := len(s) - 1; i >= 0; i-- {
		reversed[len(s)-1-i] = s[i]
	}
	return reversed
}
