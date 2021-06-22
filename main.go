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

type weaponType int

const (
	fists = iota
	blade
)

const (
	normal = "\x1b[0m"
	bold   = "\x1b[1m"
	red   = "\x1b[31m"
)

type direction string

const (
	north = "north"
	east  = "east"
	south = "south"
	west  = "west"
	up = "up"
	down = "down"
	none = "none"
)

func oppositeDirection(d direction) direction {
	if d == north {
		return south
	}
	if d == south {
		return north
	}
	if d == east {
		return west
	}
	if d == west {
		return east
	}
	if d == up {
		return down
	}
	if d == down {
		return up
	}
	return none
}

type roomType struct {
	name        string
	description string
	connections map[direction]string
	entities     []ientity
	players     []*playerCharacter
}

func (r *roomType) removePlayer(player *playerCharacter) {
	for i, it := range r.players {
		if it == player {
			r.players = append(r.players[:i], r.players[i+1:]...)
			return
		}
	}
}

func (r *roomType) messagePlayers(s string) {
	for _, p := range r.players {
		p.message(s)
	}
}

func (r *roomType) viewPlayers(player *playerCharacter) {
	for _, p := range r.players {
		if p != player {
			fmt.Fprintln(player.term, p.username, "is here.")
		}
	}
}

func (r *roomType) messagePlayersExcept(exceptPlayer *playerCharacter, s string) {
	for _, p := range r.players {
		if p != exceptPlayer {
			p.message(s)
		}
	}
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
	username      string
	location      *roomType
	grabbedEntity ientity
	ipAddr        string
	term          *terminal.Terminal
}

func (p *playerCharacter) message(s string) {
	fmt.Fprintln(p.term, s)
}

func (p *playerCharacter) log(s string) {
	log.Println(p.ipAddr, p.username, s)
}

func newPlayer(name string, location *roomType, ipAddr string, term *terminal.Terminal) *playerCharacter {
	player := &playerCharacter{name, location, nil, ipAddr, term}
	location.players = append(location.players, player)
	return player
}

type ientity interface {
	name() string
	showname() string
	pickup(player *playerCharacter, i int)
	attack(player *playerCharacter, weapon weaponType, strength int, i int)
}

type entityType struct {
	_name      string
	color     string
	canPickup bool
	canDestroy bool
}

func (e *entityType) name() string {
	return e._name
}

func (e *entityType) showname() string {
	return e.color + e._name + normal
}

func (e *entityType) pickup(player *playerCharacter, i int) {
	if e.canPickup {
		oldent := player.grabbedEntity
		if oldent != nil {
			player.location.entities = append(player.location.entities, oldent)
			player.location.messagePlayers(player.username + " drops " + oldent.name() + ".\n")
		}
		player.location.entities = append(player.location.entities[:i], player.location.entities[i+1:]...)
		player.grabbedEntity = e
		player.location.messagePlayers(player.username + " picks up " + e.name() + ".\n")
		return
	} else {
		player.message("Can't pick that up.")
		return
	}
}

func (e *entityType) attack(player *playerCharacter, weapon weaponType, strength int, i int) {
	if e.canDestroy {
		player.message("You destroyed the " + e.name() + ".")
		player.location.entities = append(player.location.entities[:i], player.location.entities[i+1:]...)
	} else {
		player.message("Can't attack that.")
	}
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

	world := newRoomCollection()
	world.addRoom(&roomType{"square", "You are in a large city square, surrounded by buildings.", map[direction]string{
		east:  "temple",
		west:  "inn",
		down: "under the square",
	}, []ientity{&entityType{_name:"merchant"}, &entityType{_name:"apple", canPickup:true, canDestroy:true}, &entityType{_name:"banana", canPickup:true, canDestroy:true}}, []*playerCharacter{}})
	world.addRoom(&roomType{"temple", "You are in a holy temple. The building is richly decorated with statues of ancient deities.", map[direction]string{
		west: "square",
	}, []ientity{&entityType{_name:"priest"}}, []*playerCharacter{}})
	world.addRoom(&roomType{"inn", "You are in a lively inn.", map[direction]string{
		east: "square",
	}, []ientity{&entityType{_name:"traveler"},&entityType{_name:"job board"}}, []*playerCharacter{}})
	world.addRoom(&roomType{"under the square", "You are in a large sewer underneath the market square", map[direction]string{
		up: "square",
	}, []ientity{&entityType{_name:"rat",color:red}}, []*playerCharacter{}})

	// player connects

	for {
		nConn, err := listener.Accept()
		if err != nil {
			panic("failed to accept incoming connection")
		}

		go func() {
			// ssh handshake
			connection, chans, reqs, err := ssh.NewServerConn(nConn, config)
			if err != nil {
				fmt.Println("failed to handshake with new client:", err)
				return
			}

			// ssh connections can make "requests" outside of the main tcp pipe
			// for the connection. receive and discard all of those.
			go ssh.DiscardRequests(reqs)

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

				// init player info
				username := connection.Permissions.Extensions["username"]
				player := newPlayer(username, world.rooms["temple"], nConn.RemoteAddr().String(), term)


				// goroutine for user input

				go func() {
					defer channel.Close()

					player.log("connected")

					fmt.Fprintf(term, "Welcome %s!\n\nYou have just entered the world of Maoxian, a multiplayer text adventure.\nType `help` (followed by enter) to see what basic commands you can perform.\n\n", username)

					fmt.Fprintln(term, player.location.description)

					player.location.messagePlayers(username + " enters the world of Maoxian...\n")
					player.location.viewPlayers(player)

					for {
						cmds := map[string]func([]string){
							"help": func(args []string) {
								fmt.Fprintln(term, "Maoxian MUD basic commands\n")

								// use tabwriter to neatly format command help
								helpWriter := tabwriter.NewWriter(term, 8, 8, 0, '\t', 0)

								commands := [][]string{
									[]string{"look or l", "Show a description of the current room and its contents."},
									[]string{"move [direction]", "Move to another room. Example: `move north` will go north."},
									[]string{"[direction]", "Alias for `move [direction]`."},
									[]string{"say [stuff...]", "Say something. People in the same room will be able to see your message."},
									[]string{"exit", "leave the land of Maoxian and return to your boring terminal."},
								}

								for _, command := range commands {
									fmt.Fprintf(helpWriter, " %s\t%s\r\n", command[0], command[1])
								}
								helpWriter.Flush()

								fmt.Fprintln(term, "\npsst! try running 'look' to get started. Remember to hit [Enter] after writing any command")
							},
							"drop": func(args []string) {
								if player.grabbedEntity != nil {
									ent := player.grabbedEntity
									player.grabbedEntity = nil
									player.location.entities = append(player.location.entities, ent)
									player.location.messagePlayers(username + " drops " + ent.name() + ".\n")
								} else {
									fmt.Fprintln(term, "You're not carrying anything!")
								}
							},
							"look": func(args []string) {
								fmt.Fprintln(term, player.location.description)
								for d, to := range player.location.connections {
									fmt.Fprintf(term, "%s%s%s => %s; ", bold, d, normal, to)
								}
								fmt.Fprintln(term, "")

								if len(player.location.entities) != 0 {
									str := "You can see: " + player.location.entities[0].showname()
									for i := 1; i < len(player.location.entities); i++ {
										str += ", " + player.location.entities[i].showname()
									}
									fmt.Fprintln(term, str)
								}

								for _, p := range player.location.players {
									fmt.Fprint(term, p.username, " is here")
									if p.grabbedEntity != nil {
										fmt.Fprint(term, ", carrying ", p.grabbedEntity.name())
									}
									fmt.Fprintln(term, ".")
								}
							},
							"move": func(args []string) {
								if len(args) == 0 {
									fmt.Fprintln(term, "type: `move [direction]` to move somewplayer.location. Example: move east.")
									return
								}

								to, exists := player.location.connections[direction(args[0])]
								if exists {
									player.location.removePlayer(player)
									player.location.messagePlayersExcept(player, username + " went " + args[0])
									player.location = world.rooms[to]
									player.location.players = append(player.location.players, player)
									fmt.Fprintf(term, "Moved to %s.\r\n", player.location.name)
									if len(player.location.entities) != 0 {
										str := "You can see: " + player.location.entities[0].showname()
										for i := 1; i < len(player.location.entities); i++ {
											str += ", " + player.location.entities[i].showname()
										}
										fmt.Fprintln(term, str)
									}
									player.location.messagePlayersExcept(player, username + " moves in from " + string(oppositeDirection(direction(args[0]))))
									player.location.viewPlayers(player)
								} else {
									fmt.Fprintln(term, "Cannot move there.")
									return
								}
							},
							"say": func(args []string) {
								if len(args) == 0 {
									fmt.Fprintln(term, "type: `say [stuff]` to say something. People in the same room will be able to see your message.")
									return
								}

								player.location.messagePlayers(username + " says \"" + strings.Join(args, " ") + "\"\n")
							},
							"take": func(args []string) {
								if len(args) == 0 {
									fmt.Fprintln(term, "type: `take [thing]` to take something in this room.")
									return
								}
								ent := strings.Join(args, " ")
								for i, it := range player.location.entities {
									if it.name() == ent {
										it.pickup(player, i)
										return
									}
								}
								fmt.Fprintln(term, "That object isn't here.")
							},
							"punch": func(args []string) {
								if len(args) == 0 {
									fmt.Fprintln(term, "type: `punch [thing]` to attack something in this room with your fists.")
									return
								}
								ent := strings.Join(args, " ")
								for i, it := range player.location.entities {
									if it.name() == ent {
										it.attack(player, fists, 0, i)
										return
									}
								}
								fmt.Fprintln(term, "That object isn't in this location.")
							},
							"whoami": func(args []string) {
								fmt.Fprintln(term, "You are", username)
							},
							"exit": func(args []string) {
								fmt.Fprintln(term, "Leaving the land of Maoxian...\r\n")

								channel.Close()
							},
						}
						// aliases
						cmds["l"] = cmds["look"]
						cmds["go"] = cmds["move"]
						cmds["quit"] = cmds["exit"]
						cmds["get"] = cmds["take"]
						cmds["north"] = func(args []string) { cmds["move"]([]string{"north"}) }
						cmds["east"] = func(args []string) { cmds["move"]([]string{"east"}) }
						cmds["south"] = func(args []string) { cmds["move"]([]string{"south"}) }
						cmds["west"] = func(args []string) { cmds["move"]([]string{"west"}) }
						cmds["down"] = func(args []string) { cmds["move"]([]string{"down"}) }
						cmds["up"] = func(args []string) { cmds["move"]([]string{"up"}) }

						line, err := term.ReadLine()
						if err != nil {
							player.log("disconnected")
							msg := username + " leaves the world of Maoxian.\n"
							player.location.removePlayer(player)
							player.location.messagePlayers(msg)
							break
						}

						player.log("ran command: " + line)

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
