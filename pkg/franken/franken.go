package franken

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

func (c *Client) Components(ctx context.Context) ([]string, error) {
	// Load components page
	resp, err := c.do(ctx, "GET", "docs/introduction", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("franken: couldn't get components: %w", err)
	}

	// Parse the document
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp))
	if err != nil {
		return nil, fmt.Errorf("franken: couldn't parse components: %w", err)
	}

	// Extract components
	var found bool
	var components []string
	doc.Find("aside.aside-l ul.uk-nav li").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if !found && text == "Components" {
			found = true
			return
		}
		if !found {
			return
		}
		href, ok := s.Find("a").Attr("href")
		if !ok {
			return
		}
		component := strings.TrimPrefix(href, "/docs/")
		components = append(components, component)
	})
	return components, nil
}

type Snippet struct {
	Component string `json:"component"`
	Title     string `json:"title"`
	HTML      string `json:"html"`
}

var ErrWIP = errors.New("franken: work in progress documentation")

func (b *Browser) Snippets(parent context.Context, component string) ([]Snippet, error) {
	// Join parent and browser contexts
	ctx, cancel := context.WithCancel(b.browserContext)
	defer cancel()
	go func() {
		select {
		case <-parent.Done():
			cancel()
		case <-ctx.Done():
		}
	}()

	// Rate limit to avoid abusing the website
	unlock := b.rateLimit.Lock(ctx)
	defer unlock()

	// Navigate to component page
	if err := chromedp.Run(ctx,
		chromedp.Navigate(fmt.Sprintf("https://www.franken-ui.dev/docs/%s", component)),
		chromedp.WaitReady("docs", chromedp.ByID),
	); err != nil {
		return nil, fmt.Errorf("franken: couldn't navigate to %s: %w", component, err)
	}

	// Wait for buttons to be ready
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("franken: context cancelled")
	case <-time.After(500 * time.Millisecond):
	}

	// Obtain the document
	var html string
	if err := chromedp.Run(ctx,
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return nil, fmt.Errorf("franken: couldn't get html: %w", err)
	}

	// Parse the document
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("franken: couldn't parse components: %w", err)
	}

	alert := doc.Find(".uk-alert .uk-paragraph").Text()
	alert = strings.ToLower(strings.TrimSpace(alert))
	if alert == "This documentation is a work in progress." {
		return nil, ErrWIP
	}

	// Click on all HTML buttons
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`Array.from(document.querySelectorAll('a')).filter(btn => btn.innerText === 'Markup').map(btn => btn.click());`, nil),
	); err != nil {
		return nil, fmt.Errorf("franken: couldn't click on HTML buttons: %w", err)
	}

	// Wait for code to be ready
	if err := chromedp.Run(ctx,
		chromedp.WaitReady(`code[data-language="html"]`, chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("franken: couldn't wait for HTML code: %w", err)
	}

	// Obtain the document
	if err := chromedp.Run(ctx,
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return nil, fmt.Errorf("franken: couldn't get html: %w", err)
	}

	// Parse the document
	doc, err = goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("franken: couldn't parse components: %w", err)
	}

	// Extract snippets
	var snippets []Snippet
	var lastTitle string
	doc.Find("#docs").Each(func(i int, s *goquery.Selection) {
		s.Contents().Each(func(j int, child *goquery.Selection) {
			if goquery.NodeName(child) == "h2" {
				text := strings.TrimSpace(child.Text())
				lastTitle = text
			}
			var code string
			child.Find(`code[data-language="html"]`).Each(func(k int, s *goquery.Selection) {
				code = s.Text()
			})
			if code == "" {
				return
			}
			snippets = append(snippets, Snippet{
				Component: component,
				Title:     lastTitle,
				HTML:      code,
			})
		})
	})
	return snippets, nil
}
