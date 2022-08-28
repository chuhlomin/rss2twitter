// Package rss implements rss event publisher. Posts new events to returned channel.
// Each event represents a new rss item
package rss

import (
	"context"
	"net/http"
	"sync"
	"time"

	log "github.com/go-pkgz/lgr"
	"github.com/mmcdole/gofeed"
	"github.com/pkg/errors"
)

// Notify on RSS change
type Notify struct {
	Feed     string
	Duration time.Duration
	Timeout  time.Duration

	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
}

// Event from RSS
type Event struct {
	ChanTitle string
	Title     string
	Link      string
	Text      string
	GUID      string
}

// Go starts notifier and returns events channel
func (n *Notify) Go(ctx context.Context) <-chan Event {
	log.Printf("[INFO] start notifier for %s, every %s", n.Feed, n.Duration)
	n.once.Do(func() { n.ctx, n.cancel = context.WithCancel(ctx) })

	ch := make(chan Event)

	// wait for duration, can be terminated by ctx
	waitOrCancel := func(ctx context.Context) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(n.Duration):
			return true
		}
	}

	go func() {

		defer func() {
			close(ch)
			n.cancel()
		}()

		fp := gofeed.NewParser()
		fp.Client = &http.Client{Timeout: n.Timeout}
		log.Printf("[DEBUG] notifier uses http timeout %v", n.Timeout)
		var seenGUIDs []string
		for {
			feedData, err := fp.ParseURL(n.Feed)
			if err != nil {
				log.Printf("[WARN] failed to fetch/parse url from %s, %v", n.Feed, err)
				if !waitOrCancel(n.ctx) {
					return
				}
				continue
			}
			events, err := n.feedEvents(feedData)
			if err != nil {
				log.Printf("[WARN] failed to parse feed %s, %v", n.Feed, err)
				continue
			}

			if len(seenGUIDs) == 0 && len(events) > 0 {
				log.Printf("[INFO] ignore first events, last is %s - %s", events[0].GUID, events[0].Title)
				for _, e := range events {
					seenGUIDs = append(seenGUIDs, e.GUID)
				}
				events = nil // effectively "goto" to waitOrCancel
			}

			for _, event := range events {
				if !inSlice(event.GUID, seenGUIDs) {
					log.Printf("[INFO] new event %s - %s", event.GUID, event.Title)
					ch <- event
				}
			}

			if !waitOrCancel(n.ctx) {
				log.Print("[WARN] notifier canceled")
				return
			}
		}
	}()

	return ch
}

// Shutdown notifier
func (n *Notify) Shutdown() {
	log.Print("[DEBUG] shutdown initiated")
	n.cancel()
	<-n.ctx.Done()
}

// feedEvent gets latest item from rss feed
func (n *Notify) feedEvents(feed *gofeed.Feed) (events []Event, err error) {
	if len(feed.Items) == 0 {
		return events, errors.New("no items in rss feed")
	}
	if feed.Items[0].GUID == "" {
		return events, errors.Errorf("no guid for rss entry %+v", feed.Items[0])
	}

	for _, item := range feed.Items {
		events = append(events, Event{
			ChanTitle: feed.Title,
			Title:     item.Title,
			Link:      item.Link,
			Text:      item.Description,
			GUID:      item.GUID,
		})
	}

	return events, nil
}

func inSlice(s string, list []string) bool {
	for _, v := range list {
		if s == v {
			return true
		}
	}
	return false
}
