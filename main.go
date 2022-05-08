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
	"sync"
	"time"
)

type Persisted struct {
	Slug  string    `json:"slug"`
	Floor float64   `json:"floor"`
	Date  time.Time `json:"date"`
}

type Config struct {
	Telegram TelegramConfig `json:"telegram"`
	Stores   []StoreConfig  `json:"stores"`
	Output   string         `json:"history_json_path"`
}

type StoreConfig struct {
	Slugs      []string `json:"collection_slugs"`
	StoreURL   string   `json:"store_url"`
	StatsURL   string   `json:"stats_url"`
	Max        float64  `json:"max"`
	Min        float64  `json:"min"`
	Tree       []string `json:"json_map"`
	Multiplier float64  `json:"multiplier"`
}

type TelegramConfig struct {
	BotID       string `json:"bot_id"`
	RecipientID string `json:"recipient_id"`
}

const TGURL = "https://api.telegram.org"

func main() {
	configPath := flag.String("c", "config.json", "config file")
	flag.Parse()
	config := parseConfig(*configPath)
	for {
		watchFloor(config)
		time.Sleep(800 * time.Millisecond)
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
	wg := new(sync.WaitGroup)
	wg.Add(len(config.Stores))

	for _, store := range config.Stores {
		// fetch collections one at a time per store
		// but fetch from many stores together
		go func(store StoreConfig) {

			for _, slug := range store.Slugs {
				url := fmt.Sprintf(store.StatsURL, slug)
				floor, err := fetchFloor(url, store.Tree, store.Multiplier)
				if err != nil {
					fmt.Println(err)
					continue
				}
				old_floor := findFloor(old_floors, slug)
				if old_floor > 0 && old_floor == floor {
					// floor unchanged. ignore
					continue
				}
				floors[slug] = floor
				fmt.Println(slug, floor)
				if floor >= store.Max || floor <= store.Min {
					// dont send message if floor is above threshold
					continue
				}
				dif := (floor - old_floor) / floor
				store_url := fmt.Sprintf(store.StoreURL, slug)
				msg := fmt.Sprintf("[%s](%s): %.4f", slug, store_url, floor)
				if dif > 0 {
					msg += fmt.Sprintf("*(+%.2f%%)*", dif*100)
				} else {
					msg += fmt.Sprintf("`(%.2f%%)`", dif*100)
				}
				message = append(message, msg)
			}
			wg.Done()
		}(store)
	}
	wg.Wait()
	if len(message) > 0 {
		err = sendMessage(config.Telegram.BotID, config.Telegram.RecipientID, strings.Join(message, "\n"))
		if err != nil {
			fmt.Println(err)
		}
	}
	if len(floors) > 0 {
		err = saveFloor(old_floors, floors, config.Output)
		if err != nil {
			fmt.Println(err)
		}
	}
}

// store
func fetchFloor(url string, tree []string, multiplier float64) (float64, error) {
	res, err := http.Get(url)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", url, err)
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", url, err)
	}
	var stats map[string]interface{}
	err = json.Unmarshal(body, &stats)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", url, err)
	}
	for _, key := range tree {
		switch val := stats[key].(type) {
		case float64:
			return val * multiplier, nil
		case map[string]interface{}:
			stats = val
		default:
			return 0, fmt.Errorf("invalid json traverse. Ended with %v", val)
		}
	}
	return 0, fmt.Errorf("%s: floor not found", url)
}

//TODO: Fetch rarity
// https://api-mainnet.magiceden.io/rpc/getListedNFTsByQueryLite?q={"$match":{"collectionSymbol":"gemmy"},"$sort":{"takerAmount":1},"$skip":0,"$limit":20,"status":[]}

// basic json persistence
func saveFloor(persisted []Persisted, floors map[string]float64, output string) error {
	for slug, floor := range floors {
		persisted = append(persisted, Persisted{slug, floor, time.Now()})
	}
	latest, err := json.Marshal(persisted)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(output, latest, 0644)
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
		return err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/bot%s/sendMessage", TGURL, bot), payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	_, err = http.DefaultClient.Do(req)
	return err
}
