package main

import (
	"testing"

	"github.com/mattn/go-mastodon"
	assert "github.com/stretchr/testify/require"
)

func TestFindMatchingStatus(t *testing.T) {
	// Because we're using fuzzy matching instead of matching a perfect string,
	// these will need to be sufficiently different for the results to be
	// correct.
	status1 := &mastodon.Status{Content: `This is the first tweet in the series and doesn't match anything.`}
	status2 := &mastodon.Status{Content: `A basic tweet that will match against the first few cases.`}
	status3 := &mastodon.Status{Content: `A tweet with Mastodon/Twitter different: https://this-should-be-a-pretty-long-link.example.com`}
	status4 := &mastodon.Status{Content: `A tweet with Mastodon/Twitter different: https://short`}

	statuses := []*mastodon.Status{status1, status2, status3, status4}

	t.Run("BasicMatch", func(t *testing.T) {
		status, distance := findMatchingStatus(
			statuses,
			&Tweet{Text: `A basic tweet that will match against the first few cases.`},
		)
		assert.Equal(t, status2, status)
		assert.Equal(t, 0, distance)
	})

	t.Run("FuzzyMatch", func(t *testing.T) {
		status, distance := findMatchingStatus(
			statuses,
			&Tweet{Text: `A basic tweet that will match against the first few cases. (fuzzy)`},
		)
		assert.Equal(t, status2, status)
		assert.Equal(t, 8, distance)
	})

	t.Run("NoMatchTooFuzzy", func(t *testing.T) {
		status, distance := findMatchingStatus(
			statuses,
			&Tweet{Text: `A basic tweet that will match against the first few cases. (fuzzy, but overly slow)`},
		)
		assert.Nil(t, status)
		assert.Equal(t, 0, distance)
	})

	t.Run("TransformedMatch", func(t *testing.T) {
		status, distance := findMatchingStatus(
			statuses,
			&Tweet{
				Text: `A tweet with Mastodon/Twitter different: https://short`,
				Entities: &TweetEntities{
					URLs: []*TweetEntitiesURL{
						{URL: "https://short", ExpandedURL: "https://this-should-be-a-pretty-long-link.example.com"},
					},
				},
			},
		)
		assert.Equal(t, status3, status)
		assert.Equal(t, 0, distance)
	})

	// Simulates a posting before a new version into effect. We should still
	// find a match by falling back to the old version.
	t.Run("TransformedMatchOldVersion", func(t *testing.T) {
		status, distance := findMatchingStatus(
			statuses,
			&Tweet{
				Text: `A tweet with Mastodon/Twitter different: https://short`,
				Entities: &TweetEntities{
					URLs: []*TweetEntitiesURL{
						{URL: "https://short", ExpandedURL: "https://this-should-be-a-pretty-long-link.example.com"},
					},
				},
			},
		)
		assert.Equal(t, status3, status)
		assert.Equal(t, 0, distance)
	})
}

func TestTootToTweet(t *testing.T) {
	assert.Equal(t,
		`RT @petervgeoghegan: Over 5 years ago my then-colleague @brandur wrote about problems with Postgres queues and the accumulation of garbage…`,
		tootToTweet(&mastodon.Status{
			Content: `<p>RT @petervgeoghegan: Over 5 years ago my then-colleague <span class="h-card"><a href="https://mastodon.social/@brandur" class="u-url mention">@<span>brandur</span></a></span> wrote about problems with Postgres queues and the accumulation of garbage…</p>`,
		}),
	)

	assert.Equal(t,
		`Nice thinking around easing Ractors into the Ruby ecosystem from @kirshatrov.

Ruby relies heavily on global state so bringing them in at the "top" will be difficult initially, but they're more amenable at the "bottom" where less state needs to be shared.

https://t.co/EF80vm1hEU`,
		tootToTweet(&mastodon.Status{
			Content: `<p>Nice thinking around easing Ractors into the Ruby ecosystem from @kirshatrov.</p><p>Ruby relies heavily on global state so bringing them in at the &quot;top&quot; will be difficult initially, but they&apos;re more amenable at the &quot;bottom&quot; where less state needs to be shared.</p><p><a href="https://t.co/EF80vm1hEU" rel="nofollow noopener noreferrer" target="_blank"><span class="invisible">https://</span><span class="">t.co/EF80vm1hEU</span><span class="invisible"></span></a></p>`,
		}),
	)

	assert.Equal(t,
		`A few romantic shots of Banff to help get your week started. Can't believe I'm still hiking in January. https://t.co/W5dsoSK8u7`,
		tootToTweet(&mastodon.Status{
			Content: `<p>A few romantic shots of Banff to help get your week started. Can&apos;t believe I&apos;m still hiking in January. <a href="https://t.co/W5dsoSK8u7" rel="nofollow noopener noreferrer" target="_blank"><span class="invisible">https://</span><span class="">t.co/W5dsoSK8u7</span><span class="invisible"></span></a></p>`,
		}),
	)
}

func TestTweetToTootV1(t *testing.T) {
	t.Run("NoOps", func(t *testing.T) {
		tweet := &Tweet{
			Text: `A tweet containing going through V1 always no-ops`,
		}
		assert.Equal(t,
			`A tweet containing going through V1 always no-ops`,
			tweetToTootV1(tweet),
		)
	})
}

func TestTweetToTootV2(t *testing.T) {
	t.Run("NoOpForBasicTweet", func(t *testing.T) {
		tweet := &Tweet{
			Text: `A tweet containing nothing interesting`,
		}
		assert.Equal(t,
			`A tweet containing nothing interesting`,
			tweetToTootV2(tweet),
		)
	})

	t.Run("ReplacesShortURLs", func(t *testing.T) {
		tweet := &Tweet{
			Text: `A tweet containing https://short1 and https://short2`,
			Entities: &TweetEntities{
				URLs: []*TweetEntitiesURL{
					{URL: "https://short1", ExpandedURL: "https://long1"},
					{URL: "https://short2", ExpandedURL: "https://long2"},
				},
			},
		}
		assert.Equal(t,
			`A tweet containing https://long1 and https://long2`,
			tweetToTootV2(tweet),
		)
	})

	t.Run("StripsTrailingURLWithMedia", func(t *testing.T) {
		tweet := &Tweet{
			Text: `A tweet containing media and an automatic link https://t.co/YuY4wvg3uM`,
			Entities: &TweetEntities{
				Medias: []*TweetEntitiesMedia{
					{Type: "photo", URL: "https://media1"},
				},
			},
		}
		assert.Equal(t,
			`A tweet containing media and an automatic link`,
			tweetToTootV2(tweet),
		)
	})

	t.Run("LeavesTrailingURLWithoutMedia", func(t *testing.T) {
		tweet := &Tweet{
			Text: `A tweet containing media and an automatic link https://t.co/YuY4wvg3uM`,
		}
		assert.Equal(t,
			`A tweet containing media and an automatic link https://t.co/YuY4wvg3uM`,
			tweetToTootV2(tweet),
		)
	})

	t.Run("AddsTwitterURLForRetweets", func(t *testing.T) {
		tweet := &Tweet{
			Text: `RT @user A tweet that's been truncated ...`,
			Retweet: &TweetRetweet{
				StatusID: 1234567890,
				User:     "user",
			},
		}
		assert.Equal(t,
			`RT @user A tweet that's been truncated ...

https://twitter.com/user/status/1234567890`,
			tweetToTootV2(tweet),
		)
	})
}
