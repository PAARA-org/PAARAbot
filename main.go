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

	// If the hamfile isn't specified, we exit here after printing a useful message.
	if *hamfile != "" {
		err := hams.ParseCallSigns(*hamfile)
		if err != nil {
			log.Fatal(err)
		} else {
			log.Println("Successfully parsed ", len(hams.CallSigns), "callsigns from ", *hamfile)
		}
	} else {
		log.Fatal("Ham file not specified. Please rerun the program with -hamfile=club_call_signs.txt or use -help for more info.")
	}

	// Check that all the Discord variables are set
	if *token == "" || *potaChannelID == "" || *sotaChannelID == "" {
		log.Fatal("Bot token, POTA or SOTA channel IDs weren't provided. Please rerun the program with these flags set or use -elp for more info.")
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
	// bot.Run()
}
