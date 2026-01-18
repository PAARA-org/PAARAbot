package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/PAARA-org/PAARAbot/bot"
	"github.com/PAARA-org/PAARAbot/buildinfo"
	"github.com/PAARA-org/PAARAbot/hams"
	"github.com/PAARA-org/PAARAbot/sota"
)

func main() {
	// Defining all the flags needed by the program
	hamfile := flag.String("hamfile", "", "File containing the list of ham callsigns to check for activations.")
	csvURL := flag.String("csvURL", "", "URL to a CSV file containing ham callsigns (e.g. Google Sheet export link).")
	refreshInterval := flag.Duration("refreshInterval", 8*time.Hour, "How often to refresh the callsigns from the CSV URL.")
	sotacsv := flag.String("sotacsv", "", "CSV file containing mapping from peak to park.")
	token := flag.String("token", "", "Discord bot token")
	potaChannelID := flag.String("potaChannelID", "", "POTA channel ID from Discord.")
	sotaChannelID := flag.String("sotaChannelID", "", "SOTA channel ID from Discord.")
	spotCheckInterval := flag.Duration("spotCheckInterval", 2*time.Minute, "How often to check for new spots")
	postThrottleTime := flag.Duration("postThrottleTime", 4*time.Hour, "How often to re-post the same spot.")
	versionFlag := flag.Bool("version", false, "Display application build information and exit.")

	// Parse the flags
	flag.Parse()

	// This is useful to tell which version is running
	if *versionFlag {
		fmt.Println("--- Application Build Info ---")
		fmt.Printf("Version:     %s\n", buildinfo.GitTag)
		fmt.Printf("Branch:      %s\n", buildinfo.GitBranch)
		fmt.Printf("Commit Hash: %s\n", buildinfo.GitCommit)
		fmt.Printf("Build Date:  %s\n", buildinfo.BuildDate)
		os.Exit(0)
	}

	var fileCallSigns []string
	// If the hamfile is specified, parse it.
	if *hamfile != "" {
		var err error
		fileCallSigns, err = hams.ParseCallSigns(*hamfile)
		if err != nil {
			log.Fatal(err)
		} else {
			log.Println("Successfully parsed ", len(fileCallSigns), "callsigns from ", *hamfile)
		}
	}

	// Function to refresh and combine callsigns
	refreshCallSigns := func() {
		var urlCallSigns []string
		if *csvURL != "" {
			var err error
			urlCallSigns, err = hams.FetchFromWeb(*csvURL)
			if err != nil {
				log.Println("Error fetching callsigns from URL:", err)
			} else {
				log.Println("Successfully fetched", len(urlCallSigns), "callsigns from URL")
			}
		}

		// Combine callsigns
		combined := make([]string, 0, len(fileCallSigns)+len(urlCallSigns))
		combined = append(combined, fileCallSigns...)
		combined = append(combined, urlCallSigns...)

		uniqueCombined := hams.Unique(combined)

		hams.SetCallSigns(uniqueCombined)
		log.Println("Total callsigns loaded:", len(uniqueCombined))
	}

	// Initial load
	refreshCallSigns()

	// Check if we have any callsigns
	if len(hams.GetCallSigns()) == 0 {
		log.Fatal("No callsigns loaded. Please provide -hamfile or -csvURL.")
	}

	// Start refresher if URL is present
	if *csvURL != "" {
		go func() {
			ticker := time.NewTicker(*refreshInterval)
			for range ticker.C {
				log.Println("Refreshing callsigns from URL...")
				refreshCallSigns()
			}
		}()
	}

	// Check that all the Discord variables are set
	if *token == "" || *potaChannelID == "" || *sotaChannelID == "" {
		log.Fatal("Bot token, POTA or SOTA channel IDs weren't provided. Please rerun the program with these flags set or use -help for more info.")
	}

	// This is an optional flag
	if *sotacsv != "" {
		sota.SotaPotaMappings = sota.ParseSotaCSV(*sotacsv)
	}

	// Set the bot's public variables with the values collected through the flags.
	bot.BotToken = *token
	bot.PotaChannelID = *potaChannelID
	bot.SotaChannelID = *sotaChannelID
	bot.RunInterval = *spotCheckInterval
	bot.ThrottleTime = *postThrottleTime

	// Let's run the bot!
	bot.Run()
}
