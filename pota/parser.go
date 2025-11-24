// This package implements a POTA spots parser.
package pota

import (
	"encoding/json"
	"io"
	"net/http"
)

type PotaSpot []struct {
	SpotID       int    `json:"spotId"`
	SpotTime     string `json:"spotTime"`
	Activator    string `json:"activator"`
	Frequency    string `json:"frequency"`
	Mode         string `json:"mode"`
	Reference    string `json:"reference"`
	Spotter      string `json:"spotter"`
	Source       string `json:"source"`
	Comments     string `json:"comments"`
	Name         string `json:"name"`
	LocationDesc string `json:"locationDesc"`
}

func ListSpots() (result PotaSpot, err error) {

	resp, err := http.Get("https://api.pota.app/spot/")
	if err != nil {
		//fmt.Println("No response from request")
		return nil, err
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body) // response body is []byte

	if err := json.Unmarshal(body, &result); err != nil { // Parse []byte to go struct pointer
		//fmt.Println("Can not unmarshal JSON")
		return nil, err
	}
	return
}
