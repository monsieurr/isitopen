package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv" // Import the godotenv library
	"github.com/nicklaw5/helix/v2"
	"golang.org/x/oauth2/clientcredentials"
)

// --- ANSI Styling Constants ---
const (
	ColorReset  = "\033[0m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	StyleBold   = "\033[1m"
)

const (
	configFile = "config.json"
	outputFile = "output.json"
)

const (
	updateInterval = 30 * time.Second
)

// --- Structs for JSON data ---

type Config struct {
	Streamers []string `json:"streamers"`
	Options   Options  `json:"options"`
}

type Options struct {
	RecordStreams bool `json:"record_streams"`
}

type StreamRecord struct {
	StreamerName    string    `json:"streamer_name"`
	Title           string    `json:"title"`
	GameName        string    `json:"game_name"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationMinutes float64   `json:"duration_minutes"`
}

// --- Global variables ---

var (
	config      Config
	helixClient *helix.Client
	liveStatus  = make(map[string]helix.Stream)
	configMutex = &sync.Mutex{}
)

// --- Terminal Management ---

func clearScreen() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		fmt.Print("\033[H\033[2J\033[3J")
	}
}

// --- Configuration Management ---
func loadConfig() error {
	configMutex.Lock()
	defer configMutex.Unlock()
	file, err := ioutil.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			config = Config{Streamers: []string{}, Options: Options{RecordStreams: false}}
			return saveConfig()
		}
		return err
	}
	return json.Unmarshal(file, &config)
}

func saveConfig() error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(configFile, data, 0644)
}

// --- Stream Recording ---
func recordStream(record StreamRecord) {
	records := []StreamRecord{}
	file, err := ioutil.ReadFile(outputFile)
	if err == nil && len(file) > 0 {
		json.Unmarshal(file, &records)
	}
	records = append(records, record)
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		log.Printf("Error marshaling output file: %v\n", err)
		return
	}
	ioutil.WriteFile(outputFile, data, 0644)
	fmt.Printf("\n[REC] Saved stream session for %s to %s\n> ", record.StreamerName, outputFile)
}

// --- Animation Logic ---

func animateHeader(stop chan struct{}) {
	animationChars := []rune{'/', '\\', 'X'}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-ticker.C:
			headerText := fmt.Sprintf("--- Twitch Stream Monitor --- (Last updated: %s) [", time.Now().Format("15:04:05"))
			spinnerColumn := len(headerText) + 1
			fmt.Printf("\033[s\033[1;%dH%c\033[u", spinnerColumn, animationChars[i%len(animationChars)])
			i++
		case <-stop:
			headerText := fmt.Sprintf("--- Twitch Stream Monitor --- (Last updated: %s) [", time.Now().Format("15:04:05"))
			spinnerColumn := len(headerText) + 1
			fmt.Printf("\033[s\033[1;%dH \033[u", spinnerColumn)
			return
		}
	}
}

// --- Core Monitoring Logic ---

func monitorStreams() {
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	checkStreamerStatus() // Initial run

	for {
		stopAnimation := make(chan struct{})
		go animateHeader(stopAnimation)

		<-ticker.C // Wait for the 30-second timer

		close(stopAnimation)              // Signal the animation to stop
		time.Sleep(50 * time.Millisecond) // Give it a moment to clean up

		checkStreamerStatus() // Fetch new data and redraw the screen
	}
}

func checkStreamerStatus() {
	configMutex.Lock()
	streamerList := make([]string, len(config.Streamers))
	copy(streamerList, config.Streamers)
	shouldRecord := config.Options.RecordStreams
	configMutex.Unlock()

	clearScreen()
	fmt.Printf("%s--- Twitch Stream Monitor --- (Last updated: %s%s%s) [ ]%s\n\n",
		ColorBlue, StyleBold, time.Now().Format("15:04:05"), ColorReset+ColorBlue, ColorReset)

	if len(streamerList) == 0 {
		fmt.Println("No streamers in the list. Use 'add <username>' to add one.")
	} else {
		resp, err := helixClient.GetStreams(&helix.StreamsParams{UserLogins: streamerList})
		if err != nil {
			fmt.Printf("Error fetching stream data: %v", err)
		} else {
			currentlyLive := make(map[string]helix.Stream)
			for _, stream := range resp.Data.Streams {
				currentlyLive[strings.ToLower(stream.UserLogin)] = stream
			}

			for userLogin, lastKnownStream := range liveStatus {
				if _, isStillLive := currentlyLive[userLogin]; !isStillLive {
					if shouldRecord {
						endedAt := time.Now()
						record := StreamRecord{
							StreamerName:    lastKnownStream.UserName,
							Title:           lastKnownStream.Title,
							GameName:        lastKnownStream.GameName,
							StartedAt:       lastKnownStream.StartedAt,
							EndedAt:         endedAt,
							DurationMinutes: endedAt.Sub(lastKnownStream.StartedAt).Minutes(),
						}
						recordStream(record)
					}
					delete(liveStatus, userLogin)
				}
			}

			for _, streamerName := range streamerList {
				userLogin := strings.ToLower(streamerName)
				stream, isLive := currentlyLive[userLogin]
				if isLive {
					if _, wasLive := liveStatus[userLogin]; !wasLive {
						liveStatus[userLogin] = stream
					}
					duration := time.Since(stream.StartedAt)
					fmt.Printf("%sO%s %s: %s%s%s [%s] (%d viewers) | Uptime: %s\n",
						ColorGreen, ColorReset, stream.UserName, StyleBold, stream.Title, ColorReset,
						stream.GameName, stream.ViewerCount, formatDuration(duration))
				} else {
					fmt.Printf("%sX%s %s is offline.\n", ColorYellow, ColorReset, streamerName)
				}
			}
		}
	}

	fmt.Printf("\n%s------------------------------------------%s\n", ColorBlue, ColorReset)
	fmt.Println("Type 'help' for commands.")
	fmt.Print("> ")
}

// --- REPL (User Interface) ---

func startREPL() {
	reader := bufio.NewReader(os.Stdin)
	for {
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		parts := strings.Fields(input)
		if len(parts) == 0 {
			fmt.Print("> ")
			continue
		}

		command := parts[0]
		args := parts[1:]

		switch command {
		case "help":
			fmt.Println("\nAvailable Commands:")
			fmt.Println("  add <username>      - Add a streamer to the monitor list.")
			fmt.Println("  remove <username>   - Remove a streamer from the list.")
			fmt.Println("  list                - Show the current list of monitored streamers.")
			fmt.Println("  toggle record       - Enable or disable recording of stream sessions.")
			fmt.Println("  options             - Show current options.")
			fmt.Println("  status              - Force an immediate status check.")
			fmt.Println("  exit, quit          - Exit the application.")
			fmt.Print("> ")

		case "add":
			if len(args) > 0 {
				configMutex.Lock()
				exists := false
				for _, s := range config.Streamers {
					if strings.EqualFold(s, args[0]) {
						exists = true
						break
					}
				}
				if !exists {
					config.Streamers = append(config.Streamers, args[0])
					saveConfig()
				}
				configMutex.Unlock()
				go checkStreamerStatus()
			} else {
				fmt.Println("Usage: add <username>")
				fmt.Print("> ")
			}

		case "remove":
			if len(args) > 0 {
				configMutex.Lock()
				target := args[0]
				var newStreamers []string
				for _, streamer := range config.Streamers {
					if !strings.EqualFold(streamer, target) {
						newStreamers = append(newStreamers, streamer)
					}
				}
				config.Streamers = newStreamers
				saveConfig()
				delete(liveStatus, strings.ToLower(target))
				configMutex.Unlock()
				go checkStreamerStatus()
			} else {
				fmt.Println("Usage: remove <username>")
				fmt.Print("> ")
			}

		case "list":
			configMutex.Lock()
			fmt.Println("\nMonitored Streamers:")
			for i, streamer := range config.Streamers {
				fmt.Printf("  %d. %s\n", i+1, streamer)
			}
			configMutex.Unlock()
			fmt.Print("> ")

		case "toggle":
			if len(args) > 0 && args[0] == "record" {
				configMutex.Lock()
				config.Options.RecordStreams = !config.Options.RecordStreams
				saveConfig()
				fmt.Printf("Stream recording is now %s.\n", boolToStatus(config.Options.RecordStreams))
				configMutex.Unlock()
			} else {
				fmt.Println("Usage: toggle record")
			}
			fmt.Print("> ")

		case "options":
			configMutex.Lock()
			fmt.Println("\nCurrent Options:")
			fmt.Printf("- Record Streams: %s\n", boolToStatus(config.Options.RecordStreams))
			configMutex.Unlock()
			fmt.Print("> ")

		case "status":
			go checkStreamerStatus()

		case "exit", "quit":
			fmt.Println("Exiting.")
			return

		default:
			fmt.Println("Unknown command. Type 'help' for a list of commands.")
			fmt.Print("> ")
		}
	}
}

// --- Main & Helpers ---
func main() {
	// --- NEW: Load variables from .env file ---
	err := godotenv.Load()
	if err != nil {
		// This is not a fatal error. It allows the app to fall back to system-wide
		// environment variables if the .env file is not present.
		log.Println("Note: .env file not found. Relying on system environment variables.")
	}
	// --- END NEW ---

	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	clientID := os.Getenv("TWITCH_CLIENT_ID")
	clientSecret := os.Getenv("TWITCH_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Fatal("TWITCH_CLIENT_ID and TWITCH_CLIENT_SECRET environment variables must be set (e.g., in a .env file).")
	}

	oauth2Config := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     "https://id.twitch.tv/oauth2/token",
	}

	helixClient, err = helix.NewClient(&helix.Options{ClientID: clientID})
	if err != nil {
		log.Fatalf("Failed to create helix client: %v", err)
	}

	token, err := oauth2Config.Token(context.Background())
	if err != nil {
		log.Fatalf("Failed to get app access token: %v", err)
	}
	helixClient.SetAppAccessToken(token.AccessToken)

	go monitorStreams()
	startREPL()
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func boolToStatus(b bool) string {
	if b {
		return "ENABLED"
	}
	return "DISABLED"
}
