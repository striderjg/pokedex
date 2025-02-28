package main

/*
	TODO:
		- Move some of the entry error handing printing to the main loop with passed up messages in error
		- Clean up my Structures
*/

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"internal/pokecache"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type cliCommand struct {
	name        string
	description string
	callback    func(*config, []string) error
}

type config struct {
	Next  string
	Prev  string
	cache *pokecache.Cache
}

const BAD_STATUS_CODE = "status not ok"

var cmds map[string]cliCommand
var ownedPokemon map[string]PokemonChar

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	cfg := config{}
	ownedPokemon = make(map[string]PokemonChar)
	initConfig(&cfg)
	initCommands()

	for {
		fmt.Print("Pokedex > ")
		scanner.Scan()
		cmdWords := cleanInput(scanner.Text())
		if len(cmdWords) == 0 {
			continue
		}
		f, ok := cmds[cmdWords[0]]
		if !ok {
			fmt.Println("Unknown command")
			continue
		}
		f.callback(&cfg, cmdWords)
	}
}

// ==================== Command Handlers ===========

func commandInspect(c *config, params []string) error {
	if len(params) < 2 {
		fmt.Println("You must pass a pokemon to inspect")
		return nil
	}
	poke, ok := ownedPokemon[params[1]]
	if !ok {
		fmt.Println("You have not caught that pokemon")
		return nil
	}

	fmt.Printf("Name: %v\n", poke.Name)
	fmt.Printf("Height: %v\n", poke.Height)
	fmt.Printf("Weight: %v\n", poke.Weight)
	fmt.Println("Stats:")
	for _, stat := range poke.Stats {
		fmt.Printf("  -%v: %v\n", stat.Stat.Name, stat.BaseStat)
	}
	fmt.Println("Types:")
	for _, t := range poke.Types {
		fmt.Printf("  -%v\n", t.Type.Name)
	}
	return nil
}

func commandPokedex(c *config, params []string) error {
	fmt.Println("Displaying contents of Pokedex")
	fmt.Println("============================================")

	for _, poke := range ownedPokemon {
		fmt.Printf(" - %v\n", poke.Name)
	}

	return nil
}

func commandCatch(c *config, params []string) error {
	const baseUrl = "https://pokeapi.co/api/v2/pokemon/"
	if len(params) < 2 {
		fmt.Println("You must pass a location to explore")
		return nil
	}
	url := baseUrl + params[1]

	data, err := getData(c, url)
	if err != nil {
		// Bad status code - probably bad pokemon name
		if strings.Compare(err.Error(), BAD_STATUS_CODE) == 0 {
			fmt.Println("Unknown Pokemon.  Please try again.")
			return nil
		}
		return err
	}

	fmt.Printf("Throwing a Pokeball at %v...\n", params[1])

	var resJson PokemonChar
	json.Unmarshal(data, &resJson)
	roll := rand.Int() % 9
	if roll > resJson.BaseExperience/20 || roll == 8 {
		if _, ok := ownedPokemon[resJson.Name]; !ok {
			ownedPokemon[resJson.Name] = resJson
			fmt.Printf("%v was caught!  Anding to pokedex!\n", resJson.Name)
		} else {
			fmt.Printf("%v was caught... but you already own a %v.  Releasing.\n", resJson.Name, resJson.Name)
		}
	} else {
		fmt.Printf("%v escaped!\n", resJson.Name)
	}
	return nil
}

func commandExplore(c *config, params []string) error {
	const baseUrl = "https://pokeapi.co/api/v2/location-area/"

	if len(params) < 2 {
		fmt.Println("You must pass a location to explore")
		return nil
	}
	url := baseUrl + params[1]
	data, err := getData(c, url)
	if err != nil {
		// Bad status code - probably bad location name
		if strings.Compare(err.Error(), BAD_STATUS_CODE) == 0 {
			fmt.Println("Location not found.  Please try again.")
			return nil
		}
		return err
	}

	var resJson PokemonList
	json.Unmarshal(data, &resJson)
	fmt.Printf("Exploring %v...\nFound Pokemon:\n", params[1])

	for _, poke := range resJson.PokemonEncounters {
		fmt.Printf(" - %v\n", poke.Pokemon.Name)
	}
	return nil
}

func commandMap(c *config, params []string) error {
	return _map(c, c.Next)
}

func commandMapb(c *config, params []string) error {
	if len(c.Prev) == 0 {
		fmt.Println("you're on the first page")
		return nil
	}
	return _map(c, c.Prev)
}

// -- internal map that takes string url for which way we're going
func _map(c *config, url string) error {
	data, err := getData(c, url)
	if err != nil {
		return err
	}

	var resJson mapJSON
	json.Unmarshal(data, &resJson)

	c.Next = resJson.Next
	c.Prev = resJson.Previous
	for _, n := range resJson.Results {
		fmt.Println(n.Name)
	}
	return nil
}

func commandExit(c *config, params []string) error {
	fmt.Println("Closing the Pokedex... Goodbye!")
	os.Exit(0)
	return nil
}

func commandHelp(c *config, params []string) error {
	helpText := "Welcome to the Pokedex!\nUsage:\n\n"

	keys := make([]string, 0, len(cmds))
	for key := range cmds {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		helpText += cmds[key].name
		helpText += ": "
		helpText += cmds[key].description + "\n"
	}

	fmt.Print(helpText)
	return nil
}

// ======================== Utility Funcs

func getData(c *config, url string) ([]byte, error) {
	data, ok := c.cache.Get(url)
	if !ok {
		//fmt.Println("Retrieving from internet")
		res, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve resource: %w", err)
		}
		defer res.Body.Close()

		if res.StatusCode > 299 || res.StatusCode < 200 {
			//return nil, fmt.Errorf(BAD_STATUS_CODE)
			return nil, errors.New(BAD_STATUS_CODE)
		}

		data, err = io.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read data: %w", err)
		}
		c.cache.Add(url, data)
	}
	return data, nil
}

func cleanInput(text string) []string {
	wordList := strings.Fields(text)
	for i, word := range wordList {
		wordList[i] = strings.ToLower(word)
	}

	return wordList
}

// ================= Init funcs ==============
func initConfig(c *config) {
	c.Next = "https://pokeapi.co/api/v2/location-area/"
	c.cache = pokecache.NewCache(5 * time.Second)

}

func initCommands() {
	cmds = map[string]cliCommand{
		"exit": {
			name:        "exit",
			description: "Exit the Pokedex",
			callback:    commandExit,
		},
		"help": {
			name:        "help",
			description: "Display a help message",
			callback:    commandHelp,
		},
		"map": {
			name:        "map",
			description: "Display consecutive locations every call",
			callback:    commandMap,
		},
		"mapb": {
			name:        "mapb",
			description: "Display the previous page of locations",
			callback:    commandMapb,
		},
		"explore": {
			name:        "explore",
			description: "Display the pokeman at location explore {location}",
			callback:    commandExplore,
		},
		"catch": {
			name:        "catch",
			description: "Attempt to catch {pokemon_name}",
			callback:    commandCatch,
		},
		"pokedex": {
			name:        "pokedex",
			description: "Display contents of Pokedex",
			callback:    commandPokedex,
		},
		"inspect": {
			name:        "inspect",
			description: "Inspect {pokeman} - Display pokemon characteristics",
			callback:    commandInspect,
		},
	}
}
