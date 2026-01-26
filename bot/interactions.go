package bot

import (
	"fmt"
	"strings"
	"sync"

	"github.com/PAARA-org/PAARAbot/pota"
	"github.com/PAARA-org/PAARAbot/sota"
	"github.com/bwmarrin/discordgo"
)

type DisplaySpot struct {
	ID        string
	Source    string
	Time      string
	Location  string
	Frequency string
	Mode      string
}

var (
	spotCache = make(map[string][]DisplaySpot)
	cacheMu   sync.RWMutex
)

// updateCache adds a spot to the cache for a callsign, avoiding duplicates.
func updateCache(callsign string, spot DisplaySpot) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	callsign = strings.ToUpper(callsign)
	spots := spotCache[callsign]

	// Check for duplicate ID
	for _, s := range spots {
		if s.ID == spot.ID {
			return // Already exists
		}
	}

	// Prepend new spot
	spots = append([]DisplaySpot{spot}, spots...)

	// Keep last 10
	if len(spots) > 10 {
		spots = spots[:10]
	}

	spotCache[callsign] = spots
}

// getCachedSpots returns a copy of cached spots for a callsign.
func getCachedSpots(callsign string) []DisplaySpot {
	cacheMu.RLock()
	defer cacheMu.RUnlock()

	if spots, ok := spotCache[callsign]; ok {
		// Return a copy
		result := make([]DisplaySpot, len(spots))
		copy(result, spots)
		return result
	}
	return nil
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if the message is in the correct channels
	if m.ChannelID != PotaChannelID && m.ChannelID != SotaChannelID {
		return
	}

	// Check if the bot is mentioned
	isMentioned := false
	for _, user := range m.Mentions {
		if user.ID == s.State.User.ID {
			isMentioned = true
			break
		}
	}

	if !isMentioned {
		return
	}

	// Extract CallSign
	// Assumes format: "@Bot CallSign" or similar.
	// We split by spaces and look for the token after the mention or just the first non-mention word.
	content := strings.TrimSpace(m.Content)
	parts := strings.Fields(content)

	var callsign string
	for _, part := range parts {
		// specific mention format <@!ID> or <@ID>
		if !strings.HasPrefix(part, "<@") {
			callsign = strings.ToUpper(part)
			break
		}
	}

	if callsign == "" {
		return // No callsign found
	}

	// Check Cache
	spots := getCachedSpots(callsign)

	// If cache is empty, fetch fresh data
	if len(spots) == 0 {
		spots = fetchFreshSpots(callsign)
	}

	if len(spots) == 0 {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("No recent spots found for %s.", callsign))
		return
	}

	// Format output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Most recent 10 spots for **%s**:\n", callsign))
	for _, spot := range spots {
		sb.WriteString(fmt.Sprintf("- **%s** [%s] %s %s %s (%s)\n", spot.Source, spot.Time, spot.Location, spot.Frequency, spot.Mode, spot.ID))
	}

	s.ChannelMessageSend(m.ChannelID, sb.String())
}

func fetchFreshSpots(callsign string) []DisplaySpot {
	var results []DisplaySpot

	// Fetch POTA
	potaSpots, err := pota.ListSpots()
	if err == nil {
		for _, v := range potaSpots {
			if strings.EqualFold(v.Activator, callsign) {
				results = append(results, DisplaySpot{
					ID:        fmt.Sprintf("POTA-%d", v.SpotID),
					Source:    "POTA",
					Time:      v.SpotTime,
					Location:  fmt.Sprintf("%s (%s %s)", v.Reference, v.Name, v.LocationDesc),
					Frequency: v.Frequency,
					Mode:      v.Mode,
				})
			}
		}
	}

	// Fetch SOTA
	sotaSpots, err := sota.ListSpots()
	if err == nil {
		for _, v := range sotaSpots {
			if strings.EqualFold(v.ActivatorCallsign, callsign) {
				freq := fmt.Sprintf("%.3fMHz", v.Frequency)
				results = append(results, DisplaySpot{
					ID:        fmt.Sprintf("SOTA-%d", v.Id),
					Source:    "SOTA",
					Time:      v.TimeStamp,
					Location:  fmt.Sprintf("%s (%s)", v.SummitCode, v.SummitName),
					Frequency: freq,
					Mode:      v.Mode,
				})
			}
		}
	}

	// Sort by time? The API usually returns sorted data.
	// But we are combining sources.
	// For simplicity, we just return what we found.
	// We can limit to 10 here too.
	if len(results) > 10 {
		results = results[:10]
	}

	return results
}
