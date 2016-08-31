package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/codegangsta/cli"
	"github.com/jhoonb/archivex"
	"github.com/nlopes/slack"
)

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	app := cli.NewApp()
	app.Name = "slack-dump"
	app.Usage = "export channel and group history to the Slack export format"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "token, t",
			Value:  "",
			Usage:  "a Slack API token: (see: https://api.slack.com/web)",
			EnvVar: "SLACK_API_TOKEN",
		},
	}
	app.Author = "Joe Fitzgerald"
	app.Email = "jfitzgerald@pivotal.io"
	app.Version = "0.0.1"
	app.Action = func(c *cli.Context) {
		token := c.String("token")
		if token == "" {
			fmt.Println("ERROR: the token flag is required...")
			fmt.Println("")
			cli.ShowAppHelp(c)
			os.Exit(2)
		}
		rooms := c.Args()
		api := slack.New(token)
		_, err := api.AuthTest()
		if err != nil {
			fmt.Println("ERROR: the token you used is not valid...")
			os.Exit(2)
		}

		// Create working directory
		dir, err := ioutil.TempDir("", "slack-dump")
		check(err)

		// Dump Users
		dumpUsers(api, dir)

		// Dump Channels and Groups
		dumpRooms(api, dir, rooms)

		archive(dir)
	}

	app.Run(os.Args)
}

func archive(dir string) {
	zip := new(archivex.ZipFile)
	pwd, err := os.Getwd()
	check(err)
	zip.Create(path.Join(pwd, "slackdump.zip"))
	zip.AddAll(dir, true)
	zip.Close()
}

// MarshalIndent is like json.MarshalIndent but applies Slack's weird JSON
// escaping rules to the output.
func MarshalIndent(v interface{}, prefix string, indent string) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return nil, err
	}

	b = bytes.Replace(b, []byte("\\u003c"), []byte("<"), -1)
	b = bytes.Replace(b, []byte("\\u003e"), []byte(">"), -1)
	b = bytes.Replace(b, []byte("\\u0026"), []byte("&"), -1)
	b = bytes.Replace(b, []byte("/"), []byte("\\/"), -1)

	return b, nil
}

func dumpUsers(api *slack.Client, dir string) {
	users, err := api.GetUsers()
	check(err)

	data, err := MarshalIndent(users, "", "    ")
	check(err)
	err = ioutil.WriteFile(path.Join(dir, "users.json"), data, 0644)
	check(err)
}

func dumpRooms(api *slack.Client, dir string, rooms []string) {
	// Dump Channels
	channels := dumpChannels(api, dir, rooms)

	// Dump Private Groups
	groups := dumpGroups(api, dir, rooms)

	if len(groups) > 0 {
		for _, group := range groups {
			channel := slack.Channel{}
			channel.ID = group.ID
			channel.Name = group.Name
			channel.Created = group.Created
			channel.Creator = group.Creator
			channel.IsArchived = group.IsArchived
			channel.IsChannel = true
			channel.IsGeneral = false
			channel.IsMember = true
			channel.LastRead = group.LastRead
			channel.Latest = group.Latest
			channel.Members = group.Members
			channel.NumMembers = group.NumMembers
			channel.Purpose = group.Purpose
			channel.Topic = group.Topic
			channel.UnreadCount = group.UnreadCount
			channel.UnreadCountDisplay = group.UnreadCountDisplay
			channels = append(channels, channel)
		}
	}

	data, err := MarshalIndent(channels, "", "    ")
	check(err)
	err = ioutil.WriteFile(path.Join(dir, "channels.json"), data, 0644)
	check(err)
}

func dumpChannels(api *slack.Client, dir string, rooms []string) []slack.Channel {
	channels, err := api.GetChannels(false)
	check(err)

	if len(rooms) > 0 {
		channels = FilterChannels(channels, func(channel slack.Channel) bool {
			for _, room := range rooms {
				if room == channel.Name {
					return true
				}
			}
			return false
		})
	}

	if len(channels) == 0 {
		var channels []slack.Channel
		return channels
	}

	for _, channel := range channels {
		dumpChannel(api, dir, channel.ID, channel.Name, "channel")
	}

	return channels
}

func dumpGroups(api *slack.Client, dir string, rooms []string) []slack.Group {
	groups, err := api.GetGroups(false)
	check(err)
	if len(rooms) > 0 {
		groups = FilterGroups(groups, func(group slack.Group) bool {
			for _, room := range rooms {
				if room == group.Name {
					return true
				}
			}
			return false
		})
	}

	if len(groups) == 0 {
		var groups []slack.Group
		return groups
	}

	for _, group := range groups {
		dumpChannel(api, dir, group.ID, group.Name, "group")
	}

	return groups
}

func dumpChannel(api *slack.Client, dir, id, name, channelType string) {
	var messages []slack.Message
	if channelType == "group" {
		messages = fetchGroupHistory(api, id)
	} else {
		messages = fetchChannelHistory(api, id)
	}

	if len(messages) == 0 {
		return
	}

	sort.Sort(byTimestamp(messages))

	currentFilename := ""
	var currentMessages []slack.Message
	for _, message := range messages {
		ts := parseTimestamp(message.Timestamp)
		filename := fmt.Sprintf("%d-%02d-%02d.json", ts.Year(), ts.Month(), ts.Day())
		if currentFilename != filename {
			writeMessagesFile(currentMessages, dir, name, currentFilename)
			currentMessages = make([]slack.Message, 0, 5)
			currentFilename = filename
		}

		currentMessages = append(currentMessages, message)
	}
	writeMessagesFile(currentMessages, dir, name, currentFilename)
}

func writeMessagesFile(messages []slack.Message, dir string, name string, filename string) {
	if len(messages) == 0 || dir == "" || name == "" || filename == "" {
		return
	}
	channelDir := path.Join(dir, name)
	err := os.MkdirAll(channelDir, 0755)
	check(err)

	data, err := MarshalIndent(messages, "", "    ")
	check(err)
	err = ioutil.WriteFile(path.Join(channelDir, filename), data, 0644)
	check(err)
}

func fetchGroupHistory(api *slack.Client, ID string) []slack.Message {
	historyParams := slack.NewHistoryParameters()
	historyParams.Count = 1000

	// Fetch History
	history, err := api.GetGroupHistory(ID, historyParams)
	check(err)
	messages := history.Messages
	latest := messages[len(messages)-1].Timestamp
	for {
		if history.HasMore != true {
			break
		}

		historyParams.Latest = latest
		history, err = api.GetGroupHistory(ID, historyParams)
		check(err)
		length := len(history.Messages)
		if length > 0 {
			latest = history.Messages[length-1].Timestamp
			messages = append(messages, history.Messages...)
		}

	}

	return messages
}

func fetchChannelHistory(api *slack.Client, ID string) []slack.Message {
	historyParams := slack.NewHistoryParameters()
	historyParams.Count = 1000

	// Fetch History
	history, err := api.GetChannelHistory(ID, historyParams)
	check(err)
	messages := history.Messages
	latest := messages[len(messages)-1].Timestamp
	for {
		if history.HasMore != true {
			break
		}

		historyParams.Latest = latest
		history, err = api.GetChannelHistory(ID, historyParams)
		check(err)
		length := len(history.Messages)
		if length > 0 {
			latest = history.Messages[length-1].Timestamp
			messages = append(messages, history.Messages...)
		}

	}

	return messages
}

func parseTimestamp(timestamp string) *time.Time {
	if utf8.RuneCountInString(timestamp) <= 0 {
		return nil
	}

	ts := timestamp

	if strings.Contains(timestamp, ".") {
		e := strings.Split(timestamp, ".")
		if len(e) != 2 {
			return nil
		}
		ts = e[0]
	}

	i, err := strconv.ParseInt(ts, 10, 64)
	check(err)
	tm := time.Unix(i, 0).Local()
	return &tm
}

// FilterGroups returns a new slice holding only
// the elements of s that satisfy f()
func FilterGroups(s []slack.Group, fn func(slack.Group) bool) []slack.Group {
	var p []slack.Group // == nil
	for _, v := range s {
		if fn(v) {
			p = append(p, v)
		}
	}
	return p
}

// FilterChannels returns a new slice holding only
// the elements of s that satisfy f()
func FilterChannels(s []slack.Channel, fn func(slack.Channel) bool) []slack.Channel {
	var p []slack.Channel // == nil
	for _, v := range s {
		if fn(v) {
			p = append(p, v)
		}
	}
	return p
}

// FilterUsers returns a new slice holding only
// the elements of s that satisfy f()
func FilterUsers(s []slack.User, fn func(slack.User) bool) []slack.User {
	var p []slack.User // == nil
	for _, v := range s {
		if fn(v) {
			p = append(p, v)
		}
	}
	return p
}
