package main

// Imports
import (
	"fmt"
	"strings"
	"log"
	"os"
	"io/ioutil"
	"path/filepath"
	"bufio"

	"github.com/BurntSushi/toml"
	"github.com/matrix-org/gomatrix"
	"github.com/andygrunwald/go-jira"
)

// tomclConfing struct
type tomlConfig struct {
	Server struct {
		Hostname string `toml:"hostname"`
		Username string `toml:"username"`
		Password string `toml:"password"`
	} `toml:"server"`
	Jira struct {
		Hostname string `toml:"hostname"`
		Username string `toml:"username"`
		Password string `toml:"password"`
	} `toml:"jira"`
}

func main() {
	// Get current path
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	run_directory := filepath.Dir(ex) + "/"
	rundirectory_os_string := filepath.ToSlash(run_directory)
	rundirectory_os_path := filepath.FromSlash(rundirectory_os_string)

	// Open logfile
	logfile, err := os.OpenFile(rundirectory_os_path + "mbot_jira.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer logfile.Close()

	// Set log output to file
	log.SetOutput(logfile)
	log.Printf("Starting mbot_jira in %v", rundirectory_os_path)
	log.Println("https://github.com/m1ndgames/mbot_jira/")

	// Parse toml config
	var config tomlConfig
	if _, err := toml.DecodeFile(rundirectory_os_path + "mbot_jira.toml", &config); err != nil {
		log.Fatal(err)
		return
	}

	// Login to Matrix server
	cli, _ := gomatrix.NewClient(config.Server.Hostname, "", "")

	resp, err := cli.Login(&gomatrix.ReqLogin{
		Type:     "m.login.password",
		User:     config.Server.Username,
		Password: config.Server.Password,
	})

	if err != nil {
		log.Fatal(err)
		return
	} else {
		log.Println(fmt.Sprintf("Successfully logged in to %+v", config.Server.Hostname))
	}

	cli.SetCredentials(resp.UserID, resp.AccessToken)

	// Login to Jira
	jiraClient, err := jira.NewClient(nil, config.Jira.Hostname)
	if err != nil {
		log.Fatal(err)
		return
	}

	res, err := jiraClient.Authentication.AcquireSessionCookie(config.Jira.Username, config.Jira.Password)
	if err != nil || res == false {
		fmt.Printf("Result: %v\n", res)
		log.Fatal(err)
		return
	} else {
		log.Println(fmt.Sprintf("Successfully logged in to %+v", config.Jira.Hostname))
	}

	// Read room DB
	roomdb, err := os.Open(rundirectory_os_path + "mbot_jira.db")
	if err != nil {
		log.Fatal(err)
		return
	}
	defer roomdb.Close()

	// Join matrix rooms
	scanner := bufio.NewScanner(roomdb)
	if scanner.Scan() == false {
		err = scanner.Err()
		if err == nil {
			log.Printf("No room found in %v", rundirectory_os_path + "mbot_jira.db")
		} else {
			log.Fatal(err)
			return
		}
	} else {
		// TODO: refactor that
		// Join room
		if _, err := cli.JoinRoom(scanner.Text(), "", nil); err != nil {
			log.Fatal(err)
		} else {
			log.Println(fmt.Sprintf("Successfully joined room %+v", scanner.Text()))
		}

		for scanner.Scan() {
			// Join room
			if _, err := cli.JoinRoom(scanner.Text(), "", nil); err != nil {
				log.Fatal(err)
			} else {
				log.Println(fmt.Sprintf("Successfully joined room %+v", scanner.Text()))
			}
		}
	}

	// Create syncer
	syncer := cli.Syncer.(*gomatrix.DefaultSyncer)

	// room member events
	syncer.OnEventType("m.room.member", func(iv *gomatrix.Event) {
		if iv.Content["membership"] == "invite" {
			if *iv.StateKey == config.Server.Username {
				log.Printf("Got invite from %v to join %v\n", iv.Sender, iv.RoomID)

				// Read the room db
				b, err := ioutil.ReadFile(rundirectory_os_path + "mbot_jira.db")
				if err != nil {
					log.Fatal(err)
					return
				}

				// Check if room is already in DB
				s := string(b)
				if strings.Contains(s, iv.RoomID) == false {
					// Add to DB
					fmt.Fprintf( roomdb,"%v\n", iv.RoomID)

					// Join room
					if _, err := cli.JoinRoom(iv.RoomID, "", nil); err != nil {
						log.Fatal(err)
					} else {
						log.Println(fmt.Sprintf("Successfully joined room %+v", iv.RoomID))
					}
				}
			}
		} else if iv.Content["membership"] == "leave" {
			if *iv.StateKey == config.Server.Username {
				log.Printf("I was kicked from %v by %v\n", iv.RoomID, iv.Sender)

				// Read the room db
				b, err := ioutil.ReadFile(rundirectory_os_path + "mbot_jira.db")
				if err != nil {
					log.Fatal(err)
					return
				}

				// Check if room is already in DB
				s := string(b)
				if strings.Contains(s, iv.RoomID) == true {
					lines := strings.Split(string(b), "\n")

					for i, line := range lines {
						if strings.Contains(line, iv.RoomID) {
							lines[i] = ""
						}
					}
					output := strings.Join(lines, "\n")
					err = ioutil.WriteFile(rundirectory_os_path + "mbot_jira.db", []byte(output), 0644)
					if err != nil {
						log.Fatalln(err)
						return
					} else {
						log.Printf("Deleted %v from room DB\n", iv.RoomID)
					}
				}
			}
		}
	})

	// message events
	syncer.OnEventType("m.room.message", func(ev *gomatrix.Event) {
		msg, _ := ev.Body()
		if strings.Contains(msg, "!jira") {
			jiraparam := strings.Fields(msg)

			// Show Jira Ticket
			if jiraparam[1] == "show" {
				issue, _, err := jiraClient.Issue.Get(jiraparam[2], nil)
				if err != nil {
					cli.SendText(ev.RoomID, "Sorry, there is no such Ticket...") // Received 404
				} else {
					// Create JSON from matrixmessage struct
					output := gomatrix.HTMLMessage{ fmt.Sprintf("%s:\n%+v", issue.Key, issue.Fields.Summary), "m.text", "org.matrix.custom.html", fmt.Sprintf("%s:\n<code><pre>%+v</code></pre>", issue.Key, issue.Fields.Summary)}

					// Send the message JSON
					cli.SendMessageEvent(ev.RoomID,"m.room.message", output)
				}
			}
		}
	})

	if err := cli.Sync(); err != nil {
		log.Println(fmt.Sprintf("Sync() returned %v", err))
	}

}
