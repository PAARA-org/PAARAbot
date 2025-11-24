package sota

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSotaCSV(t *testing.T) {
	// Setup test CSV content
	testCSV := `SummitCode,AssociationName,RegionName,SummitName,AltM,AltFt,GridRef1,GridRef2,Longitude,Latitude,Points,BonusPoints,ValidFrom,ValidTo,ActivationCount,ActivationDate,ActivationCall,ParkName,Pota
3Y/BV-001,Bouvet Island,Bouvetøya (Bouvet Island),Olavtoppen,780,2559,3.3565,-54.4104,3.3565,-54.4104,10,3,01/03/2018,31/12/2099,0,,,,
4O/IC-001,Montenegro,Istok Crne Gore,Maja Rosit,2524,8280,19.8505,42.4795,19.8505,42.4795,10,3,01/03/2019,31/12/2099,1,27/07/2022,4O/SQ9MDF/P,,
4O/IC-002,Montenegro,Istok Crne Gore,Kom kučki,2487,8159,19.6417,42.6807,19.6417,42.6807,10,3,01/03/2019,31/12/2099,0,,,,
K0M/NE-001,USA - Minnesota ,Northeast,Eagle Mountain,701,2301,-90.5605,47.8975,-90.5605,47.8975,10,3,01/10/2013,31/12/2099,13,20/06/2023,W9UUM,Superior National Forest/Boundary Waters Canoe Area Wilderness Area,US-4491
K0M/NE-002,USA - Minnesota ,Northeast,2266,691,2266,-90.3710,47.9297,-90.371,47.9297,8,3,01/10/2013,31/12/2099,0,,,Superior National Forest/Pat Bayle State Forest,US-4491/US-4816
K0M/NE-003,USA - Minnesota ,Northeast,Misquah Hills,689,2260,-90.5198,47.9749,-90.5198,47.9749,8,3,01/10/2013,31/12/2099,0,,,Superior National Forest/Boundary Waters Canoe Area Wilderness Area,US-4491
XX/XX-999,USA - Minnesota ,Northeast,Misquah Hills,689,2260,-90.5198,47.9749,-90.5198,47.9749,8,3,01/10/2013,31/12/2099,0,,,Superior National Forest/Boundary Waters Canoe Area Wilderness Area,XX-8888,,
JW/VS-597,Svalbard,Nordvest-Spitsbergen,Kobben,265,869,10.9012,79.6937,10.9012,79.6937,1,0,01/02/2021,31/12/2099,0,,,`

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.csv")
	if err := os.WriteFile(tmpFile, []byte(testCSV), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	// Execute parsing
	mappings := ParseSotaCSV(tmpFile)

	// Validate results
	t.Run("ValidPotaMapping", func(t *testing.T) {
		if mapping, exists := mappings["K0M/NE-003"]; !exists {
			t.Error("Valid POTA record not parsed")
		} else {
			if mapping.ParkId != "US-4491" || mapping.ParkName != "Superior National Forest/Boundary Waters Canoe Area Wilderness Area" {
				t.Errorf("Incorrect mapping: got %+v", mapping)
			}
		}
	})

	t.Run("MissingPotaId", func(t *testing.T) {
		if _, exists := mappings["G/LD-002"]; exists {
			t.Error("Record with empty POTA ID should be skipped")
		}
	})

	t.Run("ExtraFieldsHandling", func(t *testing.T) {
		if mapping, exists := mappings["XX/XX-999"]; !exists {
			t.Error("Record with extra fields not parsed")
		} else if mapping.ParkId != "XX-8888" {
			t.Errorf("Failed to parse record with extra fields: got %v", mapping.ParkId)
		}
	})

	t.Run("MalformedRecord", func(t *testing.T) {
		if _, exists := mappings["G/LD-004"]; exists {
			t.Error("Malformed record should be skipped")
		}
	})

	t.Run("CountValidation", func(t *testing.T) {
		if len(mappings) != 4 {
			t.Errorf("Expected 2 mappings, got %d", len(mappings))
		}
	})

	// Test error handling
	t.Run("NonexistentFile", func(t *testing.T) {
		mappings := ParseSotaCSV("nonexistent.csv")
		if len(mappings) != 0 {
			t.Error("Should return empty map for missing file")
		}
	})
}
