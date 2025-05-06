package scraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// CleanText removes extra whitespace
func cleanText(text string) string {
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(text, " "))
}

// ParseRecipeCell extracts recipes from a cell <td>
func parseRecipeCell(cell *goquery.Selection) []string {
	var recipes []string
	cell.Find("li").Each(func(i int, li *goquery.Selection) {
		text := cleanText(li.Text())
		if text != "" {
			recipes = append(recipes, text)
		}
	})
	if len(recipes) == 0 {
		text := cleanText(cell.Text())
		if text != "" {
			recipes = append(recipes, text)
		}
	}
	return recipes
}

// ElementData holds the structure of each element
type ElementData struct {
	Element string   `json:"element"`
	Recipes []string `json:"recipes"`
}

// Run scrapes and saves the elements JSON file
func Run() error {
	url := "https://little-alchemy.fandom.com/wiki/Elements_(Little_Alchemy_2)"
	res, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return fmt.Errorf("status code error: %d %s", res.StatusCode, res.Status)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return fmt.Errorf("failed to parse HTML: %w", err)
	}

	data := make(map[string][]ElementData)

	doc.Find("h3").Each(func(i int, s *goquery.Selection) {
		header := s.Find("span.mw-headline")
		if header.Length() == 0 {
			return
		}

		tierName := cleanText(header.Text())
		data[tierName] = []ElementData{}

		// Find the next <table>
		table := s.Next()
		for table != nil && goquery.NodeName(table) != "table" {
			table = table.Next()
		}
		if table == nil {
			return
		}

		table.Find("tr").Each(func(i int, row *goquery.Selection) {
			if i == 0 {
				return
			}
			cells := row.Find("td")
			if cells.Length() >= 2 {
				element := cleanText(cells.Eq(0).Text())
				recipes := parseRecipeCell(cells.Eq(1))
				data[tierName] = append(data[tierName], ElementData{
					Element: element,
					Recipes: recipes,
				})
			}
		})
	})

	file, err := os.Create("little_alchemy_elements.json")
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(data)
	if err != nil {
		return fmt.Errorf("failed to write JSON: %w", err)
	}

	fmt.Println("Scraped and saved to 'little_alchemy_elements.json'")
	return nil
}

// LoadData returns all elements as a flat list
func LoadData() ([]ElementData, error) {
	f, err := os.Open("little_alchemy_elements.json")
	if err != nil {
		return nil, errors.New("could not open little_alchemy_elements.json")
	}
	defer f.Close()

	raw := make(map[string][]ElementData)
	if err := json.NewDecoder(f).Decode(&raw); err != nil {
		return nil, errors.New("failed to parse JSON")
	}

	var all []ElementData
	for _, list := range raw {
		all = append(all, list...)
	}
	return all, nil
}
