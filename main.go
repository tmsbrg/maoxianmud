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

type room struct {
	name        string
	description string
	connections map[direction]string
	things      []string
}

func (r *room) remove(thing string) {
	r.things = removeFrom(r.things, thing)
}

type roomCollection struct {
	rooms map[string]*room
}

func newRoomCollection() roomCollection {
	return roomCollection{make(map[string]*room)}
}

func (w *roomCollection) addRoom(r *room) {
	w.rooms[r.name] = r
}

func removeFrom(slice []string, item string) []string {
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

	users := []string{}
	world := newRoomCollection()
	world.addRoom(&room{"cabin", "You are in a small cabin.", map[direction]string{east: "field"}, []string{}})
	world.addRoom(&room{"field", "You are in a large field.", map[direction]string{west: "cabin"}, []string{}})

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
			users = append(users, username)

			here := world.rooms["cabin"]
			here.things = append(here.things, username)

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

				go func() {
					defer channel.Close()

					term := terminal.NewTerminal(channel, `(ง˙o˙)ว ~> $ `)

					fmt.Fprintln(term, username, "enters the world of Maoxian...\n")
					log.Println(nConn.RemoteAddr(), username, "connected")

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

								for _, thing := range here.things {
									fmt.Fprintln(term, thing, "is here.")
								}
							},
							"move": func(args []string) {
								if len(args) == 0 {
									fmt.Fprintln(term, "type: `move [direction]` to move somewhere. Example: move east.")
									return
								}

								to, exists := here.connections[direction(args[0])]
								if exists {
									here.remove(username)
									here = world.rooms[to]
									here.things = append(here.things, username)
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
								here.remove(username)
								users = removeFrom(users, username)

								channel.Close()
							},
						}

						line, err := term.ReadLine()
						if err != nil {
							here.remove(username)
							users = removeFrom(users, username)
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
