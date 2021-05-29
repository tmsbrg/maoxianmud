package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/crypto/ssh"
	terminal "golang.org/x/term"
)

type direction string

const (
	north = "north"
	east  = "east"
	south = "south"
	west  = "west"
)

type roomType struct {
	name        string
	description string
	connections map[direction]string
	players     []int
}

func (r *roomType) removePlayer(player int) {
	r.players = removeFrom(r.players, player)
}

type roomCollection struct {
	rooms map[string]*roomType
}

func newRoomCollection() roomCollection {
	return roomCollection{make(map[string]*roomType)}
}

func (w *roomCollection) addRoom(r *roomType) {
	w.rooms[r.name] = r
}

type playerCharacter struct {
	id            int
	exists        bool
	username      string
	listenChannel chan string
}

func (p *playerCharacter) remove() {
	p.exists = false
	close(p.listenChannel)
}

var players []playerCharacter

func addPlayer(name string) int {
	channel := make(chan string, 100)
	id := len(players)
	players = append(players, playerCharacter{id, true, name, channel})
	return id
}

func removeFrom(slice []int, item int) []int {
	for i, it := range slice {
		if it == item {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

func main() {
	// init SSH connection

	var sshPort string

	envSshPort := os.Getenv("SSH_PORT")
	if envSshPort == "" {
		sshPort = ":9999"
	} else {
		sshPort = ":" + envSshPort
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			return &ssh.Permissions{Extensions: map[string]string{"username": c.User()}}, nil
		},
	}

	// create /tmp if it doesn't exist
	if _, err := os.Stat("tmp/"); os.IsNotExist(err) {
		os.Mkdir("tmp/", os.ModeDir)
	}

	privateBytes, err := ioutil.ReadFile("tmp/id_rsa")
	if err != nil {
		panic("Failed to open private key from disk. Try running `ssh-keygen` in tmp/ to create one.")
	}

	private, err := ssh.ParsePrivateKey(privateBytes)
	if err != nil {
		panic("Failed to parse private key")
	}

	config.AddHostKey(private)

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0%s", sshPort))
	if err != nil {
		panic("failed to listen for connection")
	}

	fmt.Println("SSH server running at 0.0.0.0" + sshPort)

	// init game

	players = make([]playerCharacter, 0)
	world := newRoomCollection()
	world.addRoom(&roomType{"cabin", "You are in a small cabin.", map[direction]string{east: "field"}, []int{}})
	world.addRoom(&roomType{"field", "You are in a large field.", map[direction]string{west: "cabin"}, []int{}})

	// player connects

	for {
		nConn, err := listener.Accept()
		if err != nil {
			panic("failed to accept incoming connection")
		}

		go func() {
			// ssh handshake must be performed
			connection, chans, reqs, err := ssh.NewServerConn(nConn, config)
			if err != nil {
				fmt.Println("failed to handshake with new client:", err)
				return
			}

			// ssh connections can make "requests" outside of the main tcp pipe
			// for the connection. receive and discard all of those.
			go ssh.DiscardRequests(reqs)

			username := connection.Permissions.Extensions["username"]
			player_id := addPlayer(username)

			here := world.rooms["cabin"]
			here.players = append(here.players, player_id)

			for newChannel := range chans {
				if newChannel.ChannelType() != "session" {
					newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
					continue
				}

				channel, requests, err := newChannel.Accept()
				if err != nil {
					fmt.Println("could not accept channel:", err)
					return
				}

				go func(in <-chan *ssh.Request) {
					for req := range in {
						if req.Type == "shell" {
							req.Reply(true, nil)
						}
					}
				}(requests)

				term := terminal.NewTerminal(channel, `(ง˙o˙)ว ~> $ `)

				go func(listenChannel chan string) {
					for {
						msg, ok := <-listenChannel
						if ok == false {
							return
						}
						fmt.Fprintln(term, msg)
					}
				}(players[player_id].listenChannel)

				go func() {
					defer channel.Close()

					log.Println(nConn.RemoteAddr(), username, "connected")
					msg := username + " enters the world of Maoxian...\n"
					for _, p := range players {
						if p.exists {
							p.listenChannel <- msg
						}
					}

					for {
						cmds := map[string]func([]string){
							"help": func(args []string) {
								fmt.Fprintln(term, "Maoxian MUD basic commands\n")

								// use tabwriter to neatly format command help
								helpWriter := tabwriter.NewWriter(term, 8, 8, 0, '\t', 0)

								commands := [][]string{
									[]string{"look", "Show a description of the current room and its contents"},
									[]string{"move [direction]", "Move to another room. Example: `move n` will go north."},
									[]string{"exit", "leave the land of Maoxian and return to your boring terminal."},
								}

								for _, command := range commands {
									fmt.Fprintf(helpWriter, " %s\t%s\r\n", command[0], command[1])
								}
								helpWriter.Flush()

								fmt.Fprintln(term, "\npsst! try running 'look' to get started")
							},
							"look": func(args []string) {
								fmt.Fprintln(term, here.description)

								for _, id := range here.players {
									fmt.Fprintln(term, players[id].username, "is here.")
								}
							},
							"move": func(args []string) {
								if len(args) == 0 {
									fmt.Fprintln(term, "type: `move [direction]` to move somewhere. Example: move east.")
									return
								}

								to, exists := here.connections[direction(args[0])]
								if exists {
									here.removePlayer(player_id)
									here = world.rooms[to]
									here.players = append(here.players, player_id)
									fmt.Fprintf(term, "Moved to %s.\r\n", here.name)
								} else {
									fmt.Fprintln(term, "Cannot move there.")
									return
								}
							},
							"whoami": func(args []string) {
								fmt.Fprintln(term, "You are", username)
							},
							"exit": func(args []string) {
								fmt.Fprintln(term, "Leaving the land of Maoxian...\r\n")

								channel.Close()
							},
						}

						line, err := term.ReadLine()
						if err != nil {
							log.Println(nConn.RemoteAddr(), username, "disconnected")
							here.removePlayer(player_id)
							players[player_id].remove()
							msg := username + " leaves the world of Maoxian.\n"
							for _, p := range players {
								if p.exists {
									p.listenChannel <- msg
								}
							}
							break
						}

						log.Println(nConn.RemoteAddr(), username, "ran command:", line)

						trimmedInput := strings.TrimSpace(line)

						inputElements := strings.Split(trimmedInput, " ")
						inputCmd := inputElements[0]
						inputArgs := inputElements[1:]

						if cmd, ok := cmds[inputCmd]; ok {
							fmt.Fprintln(term, "")
							cmd(inputArgs)
							fmt.Fprintln(term, "")
						} else if inputCmd != "" {
							fmt.Fprintln(term, "")
							fmt.Fprintln(term, inputCmd, "is not a known command.")
							fmt.Fprintln(term, "")
						}
					}
				}()
			}
		}()
	}
}
