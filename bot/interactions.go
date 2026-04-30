package bot

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PAARA-org/PAARAbot/pota"
	"github.com/PAARA-org/PAARAbot/sota"
	"github.com/bwmarrin/discordgo"
)

const maxCachedSpots = 10

type DisplaySpot struct {
	ID        string
	Source    string
	RawTime   time.Time
	Location  string
	Frequency string
	Mode      string
	Comment   string
}

var (
	spotCache = make(map[string][]DisplaySpot)
	cacheMu   sync.RWMutex
)

// updateCache adds a spot to the cache for a callsign. Chronologically
// consecutive duplicates (same source, location, frequency, mode and comment)
// are collapsed into a single entry whose timestamp is bumped to the latest
// observation.
func updateCache(callsign string, spot DisplaySpot) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	callsign = strings.ToUpper(callsign)
	spots := append(spotCache[callsign], spot)
	spotCache[callsign] = dedupAndSortSpots(spots, maxCachedSpots)
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

// spotsMatch reports whether two spots describe the same activation, so a
// later one is just a re-spot rather than a new entry worth recording.
func spotsMatch(a, b DisplaySpot) bool {
	return a.Source == b.Source &&
		a.Location == b.Location &&
		a.Frequency == b.Frequency &&
		a.Mode == b.Mode &&
		a.Comment == b.Comment
}

// dedupAndSortSpots sorts spots chronologically, collapses runs of consecutive
// duplicates onto the latest timestamp, and returns the most recent entries
// first, capped at limit.
func dedupAndSortSpots(spots []DisplaySpot, limit int) []DisplaySpot {
	if len(spots) == 0 {
		return spots
	}

	sort.SliceStable(spots, func(i, j int) bool {
		return spots[i].RawTime.Before(spots[j].RawTime)
	})

	deduped := make([]DisplaySpot, 0, len(spots))
	for _, s := range spots {
		if n := len(deduped); n > 0 && spotsMatch(deduped[n-1], s) {
			if s.RawTime.After(deduped[n-1].RawTime) {
				deduped[n-1].RawTime = s.RawTime
				deduped[n-1].ID = s.ID
			}
			continue
		}
		deduped = append(deduped, s)
	}

	// Reverse to newest-first.
	for i, j := 0, len(deduped)-1; i < j; i, j = i+1, j-1 {
		deduped[i], deduped[j] = deduped[j], deduped[i]
	}

	if len(deduped) > limit {
		deduped = deduped[:limit]
	}
	return deduped
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
	sb.WriteString(fmt.Sprintf("Most recent %d spots for **%s**:\n", maxCachedSpots, callsign))
	for _, spot := range spots {
		sb.WriteString(fmt.Sprintf("- **%s** [%s] %s %s %s\n", spot.Source, formatSpotTime(spot.RawTime), spot.Location, spot.Frequency, spot.Mode))
	}

	s.ChannelMessageSend(m.ChannelID, sb.String())
}

// parseRawTime parses a POTA/SOTA timestamp into a time.Time. Naive
// timestamps are assumed to be UTC. A zero time is returned on parse failure.
func parseRawTime(rawTime string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		time.RFC3339,
	}

	for _, f := range formats {
		var (
			t   time.Time
			err error
		)
		if !strings.Contains(f, "Z") && !strings.Contains(f, "-07") {
			t, err = time.ParseInLocation(f, rawTime, time.UTC)
		} else {
			t, err = time.Parse(f, rawTime)
		}
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

func formatSpotTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Local().Format("01/02 15:04")
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
					RawTime:   parseRawTime(v.SpotTime),
					Location:  fmt.Sprintf("%s (%s %s)", v.Reference, v.Name, v.LocationDesc),
					Frequency: v.Frequency,
					Mode:      v.Mode,
					Comment:   v.Comments,
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
					RawTime:   parseRawTime(v.TimeStamp),
					Location:  fmt.Sprintf("%s (%s)", v.SummitCode, v.SummitName),
					Frequency: freq,
					Mode:      v.Mode,
					Comment:   v.Comments,
				})
			}
		}
	}

	return dedupAndSortSpots(results, maxCachedSpots)
}
