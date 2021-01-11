package main

import (
	"testing"

	assert "github.com/stretchr/testify/require"
)

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
