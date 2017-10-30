package main

// Imports
import (
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/matrix-org/gomatrix"
	"github.com/andygrunwald/go-jira"
	"strings"
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
	// Parse toml config
	var config tomlConfig
	if _, err := toml.DecodeFile("config.toml", &config); err != nil {
		fmt.Println(err)
		return
	}

	// Login to Matrix server
	fmt.Printf("Connecting %+v to %+v\n", config.Server.Username, config.Server.Hostname)

	cli, _ := gomatrix.NewClient(config.Server.Hostname, "", "")

	resp, err := cli.Login(&gomatrix.ReqLogin{
		Type:     "m.login.password",
		User:     config.Server.Username,
		Password: config.Server.Password,
	})

	if err != nil {
		panic(err)
	}

	cli.SetCredentials(resp.UserID, resp.AccessToken)

	// Login to Jira
	fmt.Printf("Connecting %+v to %+v\n", config.Jira.Username, config.Jira.Hostname)

	jiraClient, err := jira.NewClient(nil, config.Jira.Hostname)
	if err != nil {
		panic(err)
	}

	res, err := jiraClient.Authentication.AcquireSessionCookie(config.Jira.Username, config.Jira.Password)
	if err != nil || res == false {
		fmt.Printf("Result: %v\n", res)
		panic(err)
	}


	// Join matrix rooms
	for _, room := range config.Server.Rooms {
		if _, err := cli.JoinRoom(room, config.Server.Hostname, nil); err != nil {
			panic(err)
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
		fmt.Println("Sync() returned ", err)
	}

}
