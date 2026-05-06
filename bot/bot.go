// This package implements a Discord bot using github.com/bwmarrin/discordgo
// and periodically fetches POTA and SOTA spots, posting messages on Discord
// when an activation is found from one of the club member ham callsigns.
package bot

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"sync"
	"time"

	"github.com/PAARA-org/PAARAbot/hams"
	"github.com/PAARA-org/PAARAbot/pota"
	"github.com/PAARA-org/PAARAbot/sota"
	"github.com/bwmarrin/discordgo"
)

// Set these public variables to allow them being set from the main package.
var BotToken string
var PotaChannelID string
var SotaChannelID string
var RunInterval time.Duration
var ThrottleTime time.Duration

// DBPath is the SQLite file used to persist the spot cache. Empty disables
// persistence and the bot runs with an in-memory cache only.
var DBPath string

type RateLimiter struct {
	mu    sync.Mutex
	users map[string]time.Time
}

func Run() {

	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lshortfile)
	logger.SetFlags(logger.Flags() | log.Llongfile)

	// Open the persistent spot store and hydrate the in-memory cache from
	// it before the first poll, so a restart inherits the previous run's
	// recent-spots view. If DBPath is unset we run memory-only.
	if DBPath != "" {
		store, err := OpenSpotStore(DBPath)
		if err != nil {
			logger.Println("Error opening spot store:", err)
			return
		}
		defer store.Close()
		if err := hydrateCacheFromStore(store); err != nil {
			logger.Println("Error loading cached spots:", err)
			return
		}
		logger.Println("Spot cache hydrated from", DBPath)
	}

	// create a session
	discord, err := discordgo.New("Bot " + BotToken)
	if err != nil {
		logger.Println("Error creating Discord session: ", err)
		return
	}

	limiter := NewRateLimiter()
	tracker := NewStatusTracker()

	discord.AddHandler(messageHandler)

	// open session
	err = discord.Open()
	if err != nil {
		logger.Println("Error opening connection:", err)
		return
	}

	defer discord.Close()

	logger.Println("Bot running....")

	// Start the message posting loop
	ticker := time.NewTicker(RunInterval)
	for range ticker.C {

		// Fetch the POTA Spots
		potaSpots, err := pota.ListSpots()
		if err != nil {
			logger.Println("Error listing POTA spots:", err)
		} else {
			logger.Println("Got ", len(potaSpots), " POTA spots.")
		}

		// Fetch the SOTA Spots
		sotaSpots, err := sota.ListSpots()
		if err != nil {
			logger.Println("Error listing SOTA spots:", err)
		} else {
			logger.Println("Got ", len(sotaSpots), " SOTA spots.")
		}

		currentCallSigns := hams.GetCallSigns()

		// Go through the POTA spots and see if any of them is for a member callsign
		for _, v := range potaSpots {
			if slices.Contains(currentCallSigns, v.Activator) {
				qrt := IsQRT(v.Comments)
				updateCache(v.Activator, DisplaySpot{
					ID:        fmt.Sprintf("POTA-%d", v.SpotID),
					Source:    "POTA",
					RawTime:   parseRawTime(v.SpotTime),
					Location:  fmt.Sprintf("%s (%s %s)", v.Reference, v.Name, v.LocationDesc),
					Frequency: v.Frequency,
					Mode:      v.Mode,
					Comment:   v.Comments,
					QRT:       qrt,
				})

				activation := fmt.Sprintf("%s at %s (%s %s)", v.Activator, v.Reference, v.Name, v.LocationDesc)
				transition := tracker.Transition("POTA|"+activation, SpotStatus{QRT: qrt})
				message := fmt.Sprintf("%s at %s (%s %s) on %sKHz %s [%s]", v.Activator, v.Reference, v.Name, v.LocationDesc, v.Frequency, v.Mode, v.Comments)
				if qrt {
					message += " QRT"
				}
				message += " \n"
				if transition || limiter.Allow(activation) {
					_, err = discord.ChannelMessageSend(PotaChannelID, message)
					if err != nil {
						fmt.Println("Error sending message:", err)
					}
				} else {
					fmt.Printf("Message throttled: %s\n", message)
				}
			}
		}
		// Do the same for the SOTA spots
		for _, v := range sotaSpots {
			if slices.Contains(currentCallSigns, v.ActivatorCallsign) {
				qrt := v.Type == "QRT" || IsQRT(v.Comments)
				updateCache(v.ActivatorCallsign, DisplaySpot{
					ID:        fmt.Sprintf("SOTA-%d", v.Id),
					Source:    "SOTA",
					RawTime:   parseRawTime(v.TimeStamp),
					Location:  fmt.Sprintf("%s (%s - %dft)", v.SummitCode, v.SummitName, v.AltFt),
					Frequency: fmt.Sprintf("%.3fMHz", v.Frequency),
					Mode:      v.Mode,
					Comment:   v.Comments,
					QRT:       qrt,
				})

				activation := fmt.Sprintf("%s at %s (%s - %dft)", v.ActivatorCallsign, v.SummitCode, v.SummitName, v.AltFt)
				transition := tracker.Transition("SOTA|"+activation, SpotStatus{QRT: qrt, Type: v.Type})
				message := fmt.Sprintf("%s at %s (%s - %dft/%dm) on %.3fMHz %s [%s]", v.ActivatorCallsign, v.SummitCode, v.SummitName, v.AltFt, v.AltM, v.Frequency, v.Mode, v.Comments)
				if qrt {
					message += " QRT"
				}
				message += " \n"
				if transition || limiter.Allow(activation) {
					_, err = discord.ChannelMessageSend(SotaChannelID, message)
					if err != nil {
						fmt.Println("Error sending message:", err)
					}
					// If this SOTA peak is in a POTA park, let's log a message too!
					r := sota.IsPota(v.SummitCode)
					if r.IsPota {
						message = fmt.Sprintf("%s at %s (%s) on %.3fMHz %s [from SOTA spot]", v.ActivatorCallsign, r.ParkId, r.ParkName, v.Frequency, v.Mode)
						if qrt {
							message += " QRT"
						}
						message += " \n"
						_, err = discord.ChannelMessageSend(PotaChannelID, message)
						if err != nil {
							fmt.Println("Error sending message:", err)
						}
					}
				} else {
					fmt.Printf("Message throttled: %s\n", message)
				}
			}
		}

	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		users: make(map[string]time.Time),
	}
}

func (rl *RateLimiter) Allow(user string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	lastAllowed, exists := rl.users[user]
	now := time.Now()

	if !exists || now.Sub(lastAllowed) >= ThrottleTime {
		rl.users[user] = now
		return true
	}
	return false
}
