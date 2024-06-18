package frankenhtml

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/igolaizola/frankenhtml/pkg/franken"
)

// Generate generates HTML files for each snippet of each frankenUI component
func Generate(ctx context.Context, output string) error {
	log.Println("running")
	defer log.Println("finished")

	// Create franken client
	client := franken.New(&franken.Config{
		Wait:  1 * time.Second,
		Debug: false,
	})

	// Get components
	components, err := client.Components(ctx)
	if err != nil {
		return err
	}
	if len(components) == 0 {
		return fmt.Errorf("no components found")
	}

	// Create browser
	browser := franken.NewBrowser(&franken.BrowserConfig{
		Wait:     1 * time.Second,
		Headless: true,
	})
	if err := browser.Start(ctx); err != nil {
		return err
	}
	defer func() { _ = browser.Stop() }()

	// Get snippets for each component
	for _, component := range components {
		snippets, err := browser.Snippets(ctx, component)
		if errors.Is(err, franken.ErrWIP) {
			log.Printf("component %q: %v\n", component, err)
			continue
		}
		if err != nil {
			return err
		}
		log.Printf("component %q (%d files)\n", component, len(snippets))
		if len(snippets) == 0 {
			continue
		}

		// Create snippets folder
		htmlFolder := filepath.Join(output, "html", component)
		if err := os.MkdirAll(htmlFolder, 0755); err != nil {
			return err
		}

		// Write snippets to files
		for _, snippet := range snippets {
			name := strings.ToLower(snippet.Title)
			name = strings.ReplaceAll(name, " ", "-")
			name = strings.ReplaceAll(name, "/", "-")
			htmlPath := filepath.Join(htmlFolder, fmt.Sprintf("%s.html", name))
			if err := os.WriteFile(htmlPath, []byte(snippet.HTML), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
