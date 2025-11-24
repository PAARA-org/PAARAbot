// This package implements a SOTA spot parser. Only spots made in the last 1 hour are retrieved.
package sota

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type SotaSpots []struct {
	Id                int     `json:"id"`
	UserID            int     `json:"userID"`
	TimeStamp         string  `json:"timeStamp"`
	Comments          string  `json:"comments"`
	Callsign          string  `json:"callsign"`
	SummitCode        string  `json:"summitCode"`
	SummitName        string  `json:"summitName"`
	ActivatorCallsign string  `json:"activatorCallsign"`
	ActivatorName     string  `json:"activatorName"`
	Frequency         float32 `json:"frequency"`
	Mode              string  `json:"mode"`
	AltM              int     `json:"AltM"`
	AltFt             int     `json:"AltFt"`
	Points            int     `json:"points"`
	Type              string  `json:"type"`
	Epoch             string  `json:"epoch"`
}

// PotaMapping stores results for POTA parks
type PotaMapping struct {
	IsPota   bool
	ParkId   string
	ParkName string
}

// sotaPota is a dictionary with key=SOTA peak and value=POTA details
type sotaPota = map[string]PotaMapping

// SotaPotaMappings is a global variable storing the dictionary with
// all the sota-pota mappings
var SotaPotaMappings = make(map[string]PotaMapping)

func ListSpots() (result SotaSpots, err error) {
	// -1 is spots in the last hour
	resp, err := http.Get("https://api2.sota.org.uk/api/spots/-1/all/all")
	if err != nil {
		//fmt.Println("No response from request")
		return result, err
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body) // response body is []byte

	if err := json.Unmarshal(body, &result); err != nil { // Parse []byte to go struct pointer
		//fmt.Println("Can not unmarshal JSON")
		return result, err
	}
	return
}

// parseSotaCSV parses a CSV file containing SOTA peaks with POTA park IDs
// and returns a dictionary with only peaks that have associated POTA parks
func ParseSotaCSV(filePath string) (mappings sotaPota) {
	fmt.Printf("Starting to parse %s\n", filePath)
	mappings = make(sotaPota)
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer file.Close()
	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable fields

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Printf("Error reading CSV: %v\n", err)
		return
	}

	// Process records starting from index 1 (skip header)
	for i, record := range records {
		if i == 0 {
			continue // Skip header row
		}

		// Ensure minimum required fields exist
		if len(record) < 19 {
			fmt.Printf("Skipping malformed record (line %d): %v\n", i+1, record)
			continue
		}
		// Only save records with POTA IDs (field index 18)
		if len(record[18]) > 0 {
			peakId := strings.TrimSpace(record[0])
			results := PotaMapping{
				IsPota:   true,
				ParkId:   strings.TrimSpace(record[18]),
				ParkName: strings.TrimSpace(record[17]),
			}
			mappings[peakId] = results
		}
	}
	fmt.Printf("Finished parsing the CSV, stored %d mappings\n", len(mappings))
	return
}

func IsPota(summitCode string) PotaMapping {
	return SotaPotaMappings[summitCode]
}
