package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Response struct {
	Stats Stats `json:"stats"`
}
type Stats struct {
	OneDayVolume          float64 `json:"one_day_volume"`
	OneDayChange          float64 `json:"one_day_change"`
	OneDaySales           float64 `json:"one_day_sales"`
	OneDayAveragePrice    float64 `json:"one_day_average_price"`
	SevenDayVolume        float64 `json:"seven_day_volume"`
	SevenDayChange        float64 `json:"seven_day_change"`
	SevenDaySales         float64 `json:"seven_day_sales"`
	SevenDayAveragePrice  float64 `json:"seven_day_average_price"`
	ThirtyDayVolume       float64 `json:"thirty_day_volume"`
	ThirtyDayChange       float64 `json:"thirty_day_change"`
	ThirtyDaySales        float64 `json:"thirty_day_sales"`
	ThirtyDayAveragePrice float64 `json:"thirty_day_average_price"`
	TotalVolume           float64 `json:"total_volume"`
	TotalSales            float64 `json:"total_sales"`
	TotalSupply           float64 `json:"total_supply"`
	Count                 float64 `json:"count"`
	NumOwners             int     `json:"num_owners"`
	AveragePrice          float64 `json:"average_price"`
	NumReports            int     `json:"num_reports"`
	MarketCap             float64 `json:"market_cap"`
	FloorPrice            float64 `json:"floor_price"`
}

type Persisted struct {
	Slug  string    `json:"slug"`
	Floor float64   `json:"floor"`
	Date  time.Time `json:"date"`
}

type Config struct {
	Telegram TelegramConfig `json:"telegram"`
	Slugs    []string       `json:"collection_slugs"`
	Output   string         `json:"history_json_path"`
	Max      float64        `json:"max"`
}

type TelegramConfig struct {
	BotID       string `json:"bot_id"`
	RecipientID string `json:"recipient_id"`
}

const STORE_URL = "https://opensea.io/collection"
const STATS_URL = "https://api.opensea.io/api/v1/collection/%s/stats"
const TGURL = "https://api.telegram.org"

func main() {
	configPath := flag.String("c", "config.json", "config file")
	flag.Parse()
	config := parseConfig(*configPath)

	for {
		watchFloor(config)
		time.Sleep(300 * time.Millisecond)
	}

}

func parseConfig(path string) Config {
	configFile, err := os.Open(path)
	if err != nil {
		log.Fatal("Cannot open server configuration file: ", err)
	}
	defer configFile.Close()

	dec := json.NewDecoder(configFile)
	var config Config
	if err = dec.Decode(&config); errors.Is(err, io.EOF) {
		//do nothing
	} else if err != nil {
		log.Fatal("Cannot load server configuration file: ", err)
	}
	return config
}

func watchFloor(config Config) {
	var message []string
	floors := map[string]float64{}
	old_floors, err := readFloor(config.Output)
	if err != nil {
		fmt.Printf("read error: %v\n", err)
		// continue anyway to generate from new fetch
	}
	// consider waitgroup
	// but might be rate limited by opensea
	for _, slug := range config.Slugs {
		stats, err := fetchStats(slug)
		if err != nil {
			fmt.Println(err)
			continue
		}
		old_floor := findFloor(old_floors, slug)

		floor := stats.FloorPrice
		if old_floor == 0 || (old_floor != floor) {
			floors[slug] = floor
			if floor > config.Max {
				// dont send message if floor is above threshold
				continue
			}
			dif := (floor - old_floor) / floor
			msg := fmt.Sprintf("[%s](%s/%s): %.4f", slug, STORE_URL, slug, floor)
			if dif > 0 {
				msg += fmt.Sprintf("*(+%.2f%%)*", dif*100)
			} else {
				msg += fmt.Sprintf("`(%.2f%%)`", dif*100)
			}
			message = append(message, msg)
		}
	}
	// TODO: wait
	if len(message) > 0 {
		sendMessage(config.Telegram.BotID, config.Telegram.RecipientID, strings.Join(message, "\n"))
	}
	if len(floors) > 0 {
		saveFloor(old_floors, floors, config.Output)
	}
}

// opensea
func fetchStats(slug string) (Stats, error) {
	var stats Stats
	res, err := http.Get(fmt.Sprintf(STATS_URL, slug))
	if err != nil {
		return stats, err
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return stats, err
	}
	var response Response
	err = json.Unmarshal(body, &response)
	if err != nil {
		return stats, err
	}
	return response.Stats, nil
}

// basic json persistence
func saveFloor(persisted []Persisted, floors map[string]float64, output string) {
	for slug, floor := range floors {
		persisted = append(persisted, Persisted{slug, floor, time.Now()})
	}
	latest, err := json.Marshal(persisted)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = ioutil.WriteFile(output, latest, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func readFloor(source string) ([]Persisted, error) {
	var floors []Persisted
	content, err := ioutil.ReadFile(source)
	if err != nil {
		return floors, err
	}
	err = json.Unmarshal(content, &floors)
	return floors, err
}

func findFloor(old []Persisted, slug string) float64 {
	for i := len(old) - 1; i >= 0; i-- {
		if old[i].Slug == slug {
			return old[i].Floor
		}
	}
	return 0
}

// telegram
func constructPayload(chatID, message string) (*bytes.Reader, error) {
	payload := map[string]interface{}{}
	payload["chat_id"] = chatID
	payload["text"] = message
	payload["parse_mode"] = "markdown"
	payload["disable_web_page_preview"] = true

	jsonValue, err := json.Marshal(payload)
	return bytes.NewReader(jsonValue), err
}

func sendMessage(bot, chatID, message string) error {
	payload, err := constructPayload(chatID, message)
	if err != nil {
		fmt.Println(err)
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/bot%s/sendMessage", TGURL, bot), payload)
	if err != nil {
		fmt.Println(err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer res.Body.Close()
	_, err = ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}
