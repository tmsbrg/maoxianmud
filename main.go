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

const (
	normal = "\x1b[0m"
	bold   = "\x1b[1m"
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
	entities     []string
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
	grabbedEntity string
	listenChannel chan string
}

func (p *playerCharacter) remove() {
	p.exists = false
	close(p.listenChannel)
}

var players []playerCharacter

func addPlayer(name string) int {
	channel := make(chan string, 100)
	for id, _ := range players {
		p := &(players[id])
		if (!p.exists) {
			p.username = name
			p.grabbedEntity = ""
			p.listenChannel = channel
			p.exists = true
			return p.id
		}
	}
	id := len(players)
	players = append(players, playerCharacter{id, true, name, "", channel})
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

type entityType struct {
	name      string
	canPickup bool
}

var entities map[string]entityType

func addEntity(name string, canPickup bool) {
	entities[name] = entityType{name, canPickup}
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
	entities = make(map[string]entityType, 0)
	addEntity("apple", true)
	addEntity("banana", true)
	addEntity("lever", false)
	addEntity("holy well", false)
	world := newRoomCollection()
	world.addRoom(&roomType{"square", "You are in a large city square, surrounded by buildings.", map[direction]string{
		//north: "residences",
		east:  "temple",
		//south: "market",
		//west:  "west street",
		down: "sewer entrance",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"temple", "You are in a holy temple. The building is richly decorated with statues of ancient deities.", map[direction]string{
		west: "square",
	}, []string{"monk", "holy well"}, []int{}})
	world.addRoom(&roomType{"sewer entrance", "You are in a large sewer underneath the market square", map[direction]string{
		up: "square",
	}, []string{"rat"}, []int{}})
	/* castle
	world.addRoom(&roomType{"castle courtyard", "You are in a courtyard surrounded by high walls.", map[direction]string{
		north: "castle lobby",
		east:  "castle east wall",
		south: "castle gate",
		west:  "castle west wall",
	}, []string{"apple", "banana"}, []int{}})
	world.addRoom(&roomType{"castle east wall", "You are on the east wall of castle Cornelia.", map[direction]string{
		south: "castle tower SE",
		west:  "castle courtyard",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle west wall", "You are on the west wall of castle Cornelia.", map[direction]string{
		south: "castle tower SW",
		east:  "castle courtyard",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle tower SE", "You are on the southeast tower of castle Cornelia. The view over Cornelia town to the east is splendid.", map[direction]string{
		north: "castle east wall",
		west:  "castle gate roof",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle tower SW", "You are on the southwest tower of castle Cornelia. The view shows a beautiful forest to the west.", map[direction]string{
		north: "castle west wall",
		east:  "castle gate roof",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle gate roof", "You are on top of the tower gate.", map[direction]string{
		east: "castle tower SE",
		west: "castle tower SW",
	}, []string{"lever"}, []int{}})
	world.addRoom(&roomType{"castle gate", "You are at the gate of castle cornelia. The gate is open.", map[direction]string{
		north: "castle courtyard",
		south: "castle road",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle road", "You are on the road to castle Cornelia. The road is surrounded by low bushes, but no trees or buildings.", map[direction]string{
		north: "castle gate",
		east:  "castle bridge",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle bridge", "You are at the bridge between castle Cornelia and the town of Cornelia. The bridge is currently undergoing construction work, and not crossable.", map[direction]string{
		west: "castle road",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle lobby", "You are in the castle lobby. There is a large staircase north and some side passenges to the side. The room look large and luxurious with red carpet and bright torches.", map[direction]string{
		north: "castle courtroom",
		east:  "castle tower NE",
		south: "castle courtyard",
		west:  "castle tower NW",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle courtroom", "You are in the court of castle Cornelia. There is a throne used by the king when making announcements.", map[direction]string{
		north: "castle royal quarters",
		east:  "castle meeting room",
		south: "castle lobby",
		west:  "castle knights quarters",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle tower NE", "You are on the northeast tower of castle Cornelia. You enjoy the fresh air and the view.", map[direction]string{
		west: "castle lobby",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle tower NW", "You are on the northwest tower of castle Cornelia. This tower has the best view over the western forest. Nature is marvelous!", map[direction]string{
		east: "castle lobby",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle meeting room", "You are a large meeting room. A massive table is at the center, surrounded by chairs. Paintings of historic events adorn all four walls. You guess 100 people could easily meet here.", map[direction]string{
		west: "castle courtroom",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle knights quarters", "You are in the knights quarters. The elite knights stay here to protect to royal family.", map[direction]string{
		east: "castle courtroom",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"castle royal quarters", "You are in the royal quarters. This room acts as the living room for the royal family. There is a fireplace, and beautiful expensive furniture. This place has a warm atmosphere.", map[direction]string{
		north: "king quarters",
		west:  "princess quarters",
		south: "castle courtroom",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"king quarters", "You are in the king's bedroom.", map[direction]string{
		south: "castle royal quarters",
	}, []string{}, []int{}})
	world.addRoom(&roomType{"princess quarters", "You are in the princess' bedroom.", map[direction]string{
		east: "castle royal quarters",
	}, []string{}, []int{}})
	*/

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

			// init player info

			username := connection.Permissions.Extensions["username"]
			player_id := addPlayer(username)

			here := world.rooms["temple"]
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

				// goroutine for printing events (other players actions, etc.)

				go func(listenChannel chan string) {
					for {
						msg, ok := <-listenChannel
						if ok == false {
							return
						}
						fmt.Fprintln(term, msg)
					}
				}(players[player_id].listenChannel)

				// goroutine for user input

				go func() {
					defer channel.Close()

					log.Println(nConn.RemoteAddr(), username, "connected")

					fmt.Fprintf(term, "Welcome %s!\n\nYou have just entered the world of Maoxian, a multiplayer text adventure.\nType `help` (followed by enter) to see what basic commands you can perform.\n\n", username)

					fmt.Fprintln(term, here.description)

					msg := username + " enters the world of Maoxian...\n"
					for _, id := range here.players {
						if id == player_id {
							continue
						}
						p := &players[id]
						p.listenChannel <- msg
						fmt.Fprintln(term, players[id].username, "is here.")
					}

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
								if players[player_id].grabbedEntity != "" {
									ent := players[player_id].grabbedEntity
									players[player_id].grabbedEntity = ""
									here.entities = append(here.entities, ent)
									for _, id := range here.players {
										players[id].listenChannel <- username + " drops " + ent + ".\n"
									}
								} else {
									fmt.Fprintln(term, "You're not carrying anything!")
								}
							},
							"look": func(args []string) {
								fmt.Fprintln(term, here.description)
								for d, to := range here.connections {
									fmt.Fprintf(term, "%s%s%s => %s; ", bold, d, normal, to)
								}
								fmt.Fprintln(term, "")
								if len(here.entities) != 0 {
									fmt.Fprintf(term, "You can see: %s\n", strings.Join(here.entities, ", "))
								}

								for _, id := range here.players {
									fmt.Fprint(term, players[id].username, " is here")
									if players[id].grabbedEntity != "" {
										fmt.Fprint(term, ", carrying ", players[id].grabbedEntity)
									}
									fmt.Fprintln(term, ".")
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
									for _, id := range here.players {
										if players[id].username != username {
											players[id].listenChannel <- username + " went " + args[0]
										}
									}
									here = world.rooms[to]
									here.players = append(here.players, player_id)
									fmt.Fprintf(term, "Moved to %s.\r\n", here.name)
									for _, id := range here.players {
										if players[id].username != username {
											fmt.Fprintln(term, players[id].username, "is here.")
											players[id].listenChannel <- username + " moves in from " + string(oppositeDirection(direction(args[0])))
										}
									}
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

								for _, id := range here.players {
									players[id].listenChannel <- username + " says \"" + strings.Join(args, " ") + "\"\n"
								}
							},
							"take": func(args []string) {
								if len(args) == 0 {
									fmt.Fprintln(term, "type: `take [thing]` to take something in this room.")
									return
								}
								ent := strings.Join(args, " ")
								for i, it := range here.entities {
									if it == ent {
										if entities[ent].canPickup {
											oldent := players[player_id].grabbedEntity
											if oldent != "" {
												here.entities = append(here.entities, oldent)
												for _, id := range here.players {
													players[id].listenChannel <- username + " drops " + oldent + ".\n"
												}
											}
											here.entities = append(here.entities[:i], here.entities[i+1:]...)
											players[player_id].grabbedEntity = ent
											for _, id := range here.players {
												players[id].listenChannel <- username + " picks up " + ent + ".\n"
											}
											return
										} else {
											fmt.Fprintln(term, "Can't pick that up.")
											return
										}
									}
								}
								fmt.Fprintln(term, "That object isn't here.")
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

						line, err := term.ReadLine()
						if err != nil {
							log.Println(nConn.RemoteAddr(), username, "disconnected")
							msg := username + " leaves the world of Maoxian.\n"
							here.removePlayer(player_id)
							players[player_id].remove()
							for _, id := range here.players {
								p := &players[id]
								p.listenChannel <- msg
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
