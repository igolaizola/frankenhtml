package franken

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/go-rod/stealth"
	"github.com/igolaizola/frankenhtml/pkg/ratelimit"
)

type Browser struct {
	parent           context.Context
	browserContext   context.Context
	allocatorContext context.Context
	browserCancel    context.CancelFunc
	allocatorCancel  context.CancelFunc
	rateLimit        ratelimit.Lock
	remote           string
	proxy            string
	profile          bool
	binPath          string
	headless         bool
}

type BrowserConfig struct {
	Wait     time.Duration
	Remote   string
	Proxy    string
	Profile  bool
	BinPath  string
	Headless bool
}

func NewBrowser(cfg *BrowserConfig) *Browser {
	wait := cfg.Wait
	if wait == 0 {
		wait = 1 * time.Second
	}
	return &Browser{
		remote:    cfg.Remote,
		proxy:     cfg.Proxy,
		profile:   cfg.Profile,
		rateLimit: ratelimit.New(wait),
		binPath:   cfg.BinPath,
		headless:  cfg.Headless,
	}
}

func (b *Browser) Start(parent context.Context) error {
	var browserContext, allocatorContext context.Context
	var browserCancel, allocatorCancel context.CancelFunc

	// Create a new context
	if b.remote != "" {
		log.Println("franken: connecting to browser at", b.remote)
		allocatorContext, allocatorCancel = chromedp.NewRemoteAllocator(context.Background(), b.remote)
	} else {
		log.Println("franken: launching browser")
		opts := append(
			chromedp.DefaultExecAllocatorOptions[3:],
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,
			chromedp.Flag("headless", b.headless),
		)

		if b.binPath != "" {
			opts = append(opts,
				chromedp.ExecPath(b.binPath),
			)
		}

		if b.proxy != "" {
			opts = append(opts,
				chromedp.ProxyServer(b.proxy),
			)
		}

		if b.profile {
			opts = append(opts,
				// if user-data-dir is set, chrome won't load the default profile,
				// even if it's set to the directory where the default profile is stored.
				// set it to empty to prevent chromedp from setting it to a temp directory.
				chromedp.UserDataDir(""),
				chromedp.Flag("disable-extensions", false),
			)
		}
		allocatorContext, allocatorCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	}

	// create chrome instance
	browserContext, browserCancel = chromedp.NewContext(
		allocatorContext,
		// chromedp.WithDebugf(log.Printf),
	)

	// Cancel the browser context when the parent is done
	go func() {
		select {
		case <-parent.Done():
		case <-browserContext.Done():
			return
		}
		browserCancel()
		allocatorCancel()
	}()

	// Launch stealth plugin
	if err := chromedp.Run(
		browserContext,
		chromedp.Evaluate(stealth.JS, nil),
	); err != nil {
		return fmt.Errorf("franken: could not launch stealth plugin: %w", err)
	}

	// disable webdriver
	if err := chromedp.Run(browserContext, chromedp.ActionFunc(func(cxt context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument("Object.defineProperty(navigator, 'webdriver', { get: () => false, });").Do(cxt)
		if err != nil {
			return err
		}
		return nil
	})); err != nil {
		return fmt.Errorf("could not disable webdriver: %w", err)
	}

	if err := chromedp.Run(browserContext,
		// Load google first to have a sane referer
		chromedp.Navigate("https://www.google.com/"),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Navigate("https://www.franken-ui.dev/docs/introduction"),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		return fmt.Errorf("franken: couldn't navigate: %w", err)
	}

	// Obtain the document
	var html string
	if err := chromedp.Run(browserContext,
		chromedp.OuterHTML("html", &html),
	); err != nil {
		return fmt.Errorf("franken: couldn't get html: %w", err)
	}

	// TODO: Search for something in the document to check if it's the correct page

	b.browserContext = browserContext
	b.browserCancel = browserCancel
	b.allocatorContext = allocatorContext
	b.allocatorCancel = allocatorCancel
	b.parent = parent

	return nil
}

// Stop closes the browser.
func (c *Browser) Stop() error {
	defer func() {
		c.browserCancel()
		c.allocatorCancel()
		go func() {
			_ = chromedp.Cancel(c.browserContext)
		}()
	}()
	return nil
}
