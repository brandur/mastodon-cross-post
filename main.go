package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/joeshaw/envdecode"
	"github.com/mattn/go-mastodon"
	"github.com/pelletier/go-toml"
)

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Main
//
//
//
//////////////////////////////////////////////////////////////////////////////

func main() {
	if len(os.Args) != 2 {
		die(fmt.Sprintf("usage: %s <Twitter TOML data file>", os.Args[0]))
	}
	source := os.Args[1]

	var conf Conf
	if err := envdecode.Decode(&conf); err != nil {
		die(fmt.Errorf("error decoding conf from env: %v", err).Error())
	}

	client := mastodon.NewClient(&mastodon.Config{
		AccessToken: conf.MastodonAccessToken,
		Server:      conf.MastodonServerURL,
	})

	err := syncTwitterToMastodon(context.Background(), &conf, client, source)
	if err != nil {
		die(fmt.Sprintf("error syncing: %v", err))
	}
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Types
//
//
//
//////////////////////////////////////////////////////////////////////////////

// Conf contains the program's configuration as specified through environmental
// variables.
type Conf struct {
	DryRun bool `env:"DRY_RUN,required"`

	MastodonAccessToken string `env:"MASTODON_ACCESS_TOKEN,required"`
	MastodonServerURL   string `env:"MASTODON_SERVER_URL,required"`

	// MaxTweetsToSync is the maximum number of tweets to post in a single run.
	// This helps space things out a bit when syncing over a large number of
	// tweets.
	MaxTweetsToSync int `env:"MAX_TWEETS_TO_SYNC,required"`

	// MinTweetID is the Twitter 64-bit integer ID of the tweet to start to try
	// and sync from. The idea is that we're not going to go back all the way
	// into ancient history, and rather start posting from some more recent
	// content only.
	MinTweetID int64 `env:"MIN_TWEET_ID,required"`
}

//
// Twitter
//

// TweetDB is a database of tweets stored to a TOML file.
type TweetDB struct {
	Tweets []*Tweet `toml:"tweets"`
}

// Tweet is a single tweet stored to a TOML file.
type Tweet struct {
	CreatedAt     time.Time      `toml:"created_at"`
	Entities      *TweetEntities `toml:"entities"`
	FavoriteCount int            `toml:"favorite_count,omitempty"`
	ID            int64          `toml:"id"`
	Reply         *TweetReply    `toml:"reply"`
	Retweet       *TweetRetweet  `toml:"retweet"`
	RetweetCount  int            `toml:"retweet_count,omitempty"`
	Text          string         `toml:"text"`
}

// TweetEntities contains various multimedia entries that may be contained in a
// tweet.
type TweetEntities struct {
	Medias       []*TweetEntitiesMedia       `toml:"medias"`
	URLs         []*TweetEntitiesURL         `toml:"urls"`
	UserMentions []*TweetEntitiesUserMention `toml:"user_mentions"`
}

// TweetEntitiesMedia is an image or video stored in a tweet.
type TweetEntitiesMedia struct {
	ID   int64  `toml:"id"`
	Type string `toml:"type"`
	URL  string `toml:"url"`
}

// TweetEntitiesURL is a URL referenced in a tweet.
type TweetEntitiesURL struct {
	DisplayURL  string `toml:"display_url"`
	ExpandedURL string `toml:"expanded_url"`
	URL         string `toml:"url"`
}

// TweetEntitiesUserMention is another user being mentioned in a tweet.
type TweetEntitiesUserMention struct {
	User   string `toml:"user"`
	UserID int64  `toml:"user_id"`
}

// TweetReply is populated with reply information for when a tweet is a
// reply.
type TweetReply struct {
	StatusID int64  `toml:"status_id"`
	User     string `toml:"user"`
	UserID   int64  `toml:"user_id"`
}

// TweetRetweet is populated with retweet information for when a tweet is a
// retweet.
type TweetRetweet struct {
	StatusID int64  `toml:"status_id"`
	User     string `toml:"user"`
	UserID   int64  `toml:"user_id"`
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Variables
//
//
//
//////////////////////////////////////////////////////////////////////////////

var logger = &LeveledLogger{Level: LevelInfo}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Private functions
//
//
//
//////////////////////////////////////////////////////////////////////////////

func die(message string) {
	fmt.Fprintf(os.Stderr, message)
	os.Exit(1)
}

func syncTweetToMastodon(ctx context.Context, conf *Conf, client *mastodon.Client, tweet *Tweet) error {
	tweetSample := tweet.Text
	if len(tweetSample) > 50 {
		tweetSample = tweetSample[0:49] + " ..."
		tweetSample = strings.Replace(tweetSample, "\n", " ", -1)
	}

	if conf.DryRun {
		logger.Infof("Would have published tweet: %s", tweetSample)
	} else {
		status, err := client.PostStatus(ctx, &mastodon.Toot{
			Status: tweet.Text,
		})
		if err != nil {
			return fmt.Errorf("error posting status: %w", err)
		}

		logger.Infof("Posted status: %v (%s)", status.ID, tweetSample)
	}

	return nil
}

func syncTwitterToMastodon(ctx context.Context, conf *Conf, client *mastodon.Client, source string) error {
	existingData, err := ioutil.ReadFile(source)
	if err != nil {
		return fmt.Errorf("error reading source twitter data file: %w", err)
	}

	var existingTweetDB TweetDB
	err = toml.Unmarshal(existingData, &existingTweetDB)
	if err != nil {
		return fmt.Errorf("error unmarshaling toml: %w", err)
	}

	var tweetCandidates []*Tweet
	for _, tweet := range existingTweetDB.Tweets {
		// Assume the file is ordered by descending tweet ID
		if tweet.ID < conf.MinTweetID {
			break
		}

		// Don't include replies are @'s
		if tweet.Reply != nil || strings.HasSuffix(tweet.Text, "@") {
			continue
		}

		tweetCandidates = append(tweetCandidates, tweet)
	}
	logger.Infof("Found %v candidate(s) for syncing to Mastodon", len(tweetCandidates))

	account, err := client.GetAccountCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("error getting current user account: %w", err)
	}

	logger.Infof("Mastadon account ID: %v", account.ID)

	statuses, err := client.GetAccountStatuses(ctx, account.ID, nil)
	if err != nil {
		return fmt.Errorf("error getting statuses: %w", err)
	}
	logger.Infof("Found %v existing status(es)", len(statuses))

	var tweetsToSync []*Tweet

	// Iterate in reverse order
	for i := len(tweetCandidates) - 1; i >= 0; i-- {
		tweet := tweetCandidates[i]

		var matchingStatus *mastodon.Status
		for _, status := range statuses {
			if status.Content == tweet.Text {
				matchingStatus = status
				break
			}
		}

		if matchingStatus == nil {
			tweetsToSync = append(tweetsToSync, tweet)
		} else {
			logger.Infof("Found content match for tweet %v in Mastodon status %v",
				tweet.ID, matchingStatus.ID)
		}
	}

	logger.Infof("Found %v tweet(s) to sync to Mastodon", len(tweetsToSync))

	// Note these end up in chronological order, meaning we tweet the oldest
	// tweets first, which is expected.
	for i, tweet := range tweetsToSync {
		if i >= conf.MaxTweetsToSync {
			logger.Infof("Hit maximum number of tweets to sync (%v); breaking",
				conf.MaxTweetsToSync)
			break
		}

		err := syncTweetToMastodon(ctx, conf, client, tweet)
		if err != nil {
			return fmt.Errorf("error syncing tweet: %w", err)
		}
	}

	return nil
}
