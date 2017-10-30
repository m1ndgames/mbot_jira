package main

// Imports
import (
	"fmt"
	"strings"
	"log"
	"os"

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
		Rooms []string `toml:"rooms"`
	} `toml:"server"`
	Jira struct {
		Hostname string `toml:"hostname"`
		Username string `toml:"username"`
		Password string `toml:"password"`
	} `toml:"jira"`
}

func main() {
	// Open logfile
	logfile, err := os.OpenFile("mbot_jira.log", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}

	// Set log output to file
	log.SetOutput(logfile)
	log.Println("Starting mbot_jira")

	// Parse toml config
	var config tomlConfig
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		fmt.Println(err)
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
	} else {
		log.Println(fmt.Sprintf("Successfully logged in to %+v", config.Server.Hostname))
	}

	cli.SetCredentials(resp.UserID, resp.AccessToken)

	// Login to Jira
	jiraClient, err := jira.NewClient(nil, config.Jira.Hostname)
	if err != nil {
		log.Fatal(err)
	}

	res, err := jiraClient.Authentication.AcquireSessionCookie(config.Jira.Username, config.Jira.Password)
	if err != nil || res == false {
		fmt.Printf("Result: %v\n", res)
		log.Fatal(err)
	} else {
		log.Println(fmt.Sprintf("Successfully logged in to %+v", config.Jira.Hostname))
	}


	// Join matrix rooms
	for _, room := range config.Server.Rooms {
		if _, err := cli.JoinRoom(room, config.Server.Hostname, nil); err != nil {
			log.Fatal(err)
		} else {
			log.Println(fmt.Sprintf("Successfully joined room %+v", room))
		}
	}

	syncer := cli.Syncer.(*gomatrix.DefaultSyncer)
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
