package main

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

/* ------------------------------------------------------ */

type User struct {
	Username string
	Password string
}

type Session struct {
	User User
	Conn *websocket.Conn
}

/* ------------------------------------------------------ */

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

/* ------------------------------------------------------ */

/*
`clients` Stores each client connection
NOTE: It's important to remove the client connection once it's closed. It is also important to remove the session.
*/

var clients []*websocket.Conn
var sessions []*Session
var users []User = []User{
	{
		Username: "alvaro",
		Password: "alvaro123",
	},
	{
		Username: "mononon",
		Password: "123",
	},
}

/* ------------------------------------------------------ */

func closeSession(session *Session) error {
	for i, eachSession := range sessions {
		if eachSession == session {
			// Removing session from slice
			sessions = append(sessions[:i], sessions[i+1:]...)
			return nil
		}
	}
	return errors.New("error when closing client session")
}

/*
closeClient function first removes the session then disconnects the client
*/
func closeClient(clientConn *websocket.Conn) error {
	// Removes the client conn from the client slice
	for i, eachCliConn := range clients {
		if eachCliConn == clientConn {
			clients = append(clients[:i], clients[i+1:]...)
		} else if len(clients)-1 == i {
			// Executes if it is the last cicle and the connection wasn't found
			return errors.New("unknown client connection")
		}
	}
	// Closes any session asociated to the connection
	cliSession := findSessionPerConn(clientConn)
	if cliSession != nil {
		if err := closeSession(cliSession); err != nil {
			return err
		}
	}
	// Closes the connection
	(*clientConn).Close()
	return nil
}

/* ------------------------------------------------------ */

func findSessionPerConn(cliConn *websocket.Conn) *Session {
	for _, eachSession := range sessions {
		if eachSession.Conn == cliConn {
			return eachSession
		}
	}
	return nil
}

func findSessionPerUsrname(usrname string) *Session {
	for _, eachSession := range sessions {
		if eachSession.User.Username == usrname {
			return eachSession
		}
	}
	return nil
}

/* ------------------------------------------------------ */

func authenticate(usrname string, passwd string) bool {
	for _, eachUser := range users {
		if eachUser.Username == usrname && eachUser.Password == passwd {
			return true
		}
	}
	return false
}

/* ------------------------------------------------------ */

func main() {
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println(err)
			return
		}

		clients = append(clients, conn)

		// When the handler function ends the connection it's been close
		defer func() {
			if err := closeClient(conn); err != nil {
				fmt.Println(err)
				return
			}
		}()

		for {
			// Reads messages from any client
			_, msg, err := conn.ReadMessage()
			if err != nil {
				// In case of the client closes the connection, the infinite loop will stop and the deffered function will execute removing the client session (if the cli has any session)
				fmt.Println(err)
				break
			}

			message := string(msg)

			// Client is entering a command
			if string(msg[0]) == "/" {
				var command string = strings.TrimPrefix(message, "/")

				if command == "exit" {
					/* Command used for closing the session and the connection */
					break
				} else if command == "list" {
					/* Command used for showing all the client connections */

					// Lists all the clients
					responseMessage := "Connected clients:\n"
					for _, eachCliConn := range clients {
						responseMessage += fmt.Sprintf("%v\n", eachCliConn.RemoteAddr())
					}
					if err := conn.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
						fmt.Println(err)
						break
					}
				} else if strings.Split(command, " ")[0] == "login" {
					/* Command used for loggin */

					// Checks if the client is logged in
					sessionIsAtive := findSessionPerConn(conn) != nil
					if sessionIsAtive {
						if err := conn.WriteMessage(websocket.TextMessage, []byte("You are already logged in")); err != nil {
							fmt.Println(err)
							break
						}
					} else {
						// There isn't any session active for the client conn

						// Arguments for the command
						args := strings.Split(command, " ")
						if len(args) != 3 {
							if err := conn.WriteMessage(websocket.TextMessage, []byte("Invalid command")); err != nil {
								fmt.Println(err)
								break
							}
						}
						usrname := args[1]
						password := args[2]

						sessionIsActive := findSessionPerUsrname(usrname)
						if sessionIsActive != nil {
							if err := conn.WriteMessage(websocket.TextMessage, []byte("Session already active")); err != nil {
								fmt.Println(err)
								break
							}
						} else {
							// Checks if the user exists
							auth := authenticate(usrname, password)

							// Adds a new session if auth
							if auth {
								newSession := &Session{Conn: conn, User: User{Username: usrname, Password: password}}
								sessions = append(sessions, newSession)
								if err := conn.WriteMessage(websocket.TextMessage, []byte("Logged as "+usrname)); err != nil {
									fmt.Println(err)
									break
								}
								// Warns all the clients that a new client has joined
								var errWhenWritingMsg error = nil
								for _, client := range clients {
									if err := client.WriteMessage(websocket.TextMessage, []byte(usrname+" joined to the server")); err != nil {
										fmt.Println(err)
										errWhenWritingMsg = err
									}
								}
								// Closes all connections with clients in case there was an error sending to each client the joining message
								if errWhenWritingMsg != nil {
									break
								}
							} else {
								if err := conn.WriteMessage(websocket.TextMessage, []byte("Invalid credentials")); err != nil {
									fmt.Println(err)
									break
								}
							}

						}

					}
				} else if command == "logout" {
					currentSession := findSessionPerConn(conn)
					if currentSession == nil {
						// There isn't any session active so the client can't logout
						if err := conn.WriteMessage(websocket.TextMessage, []byte("You are not logged in")); err != nil {
							fmt.Println(err)
							break
						}
					} else {
						// Removes the session
						var errWhenWritingMsg error = nil
						for _, cliConn := range clients {
							if err := cliConn.WriteMessage(websocket.TextMessage, []byte(currentSession.User.Username+" left the server")); err != nil {
								fmt.Println(err)
								errWhenWritingMsg = err
							}
						}
						// Closes all connections with clients in case there was an error sending to each client the joining message
						if errWhenWritingMsg != nil {
							break
						}

						if err := closeSession(currentSession); err != nil {
							if err := conn.WriteMessage(websocket.TextMessage, []byte(err.Error())); err != nil {
								fmt.Println(err)
								break
							}
						}
					}
				} else if command == "sessions" {
					// Lists all the sessions
					responseMessage := "Sessions:\n"
					for _, eachSession := range sessions {
						responseMessage += fmt.Sprintf("%v - %v\n", eachSession.User.Username, eachSession.Conn.RemoteAddr())
					}
					if err := conn.WriteMessage(websocket.TextMessage, []byte(responseMessage)); err != nil {
						fmt.Println(err)
						break
					}
				} else {
					// The command doesn't exist
					if err := conn.WriteMessage(websocket.TextMessage, []byte("Unknown command")); err != nil {
						fmt.Println(err)
						break
					}
				}
			} else {
				// Cheks if the client connection has a session
				currentSession := findSessionPerConn(conn)
				if currentSession == nil {
					// Reject the command
					if err := conn.WriteMessage(websocket.TextMessage, []byte("You are not logged in")); err != nil {
						fmt.Println(err)
						break
					}
				} else {
					for _, eachCliConn := range clients {
						sessionUsrName := (*currentSession).User.Username
						msgWithSession := fmt.Sprintf("[%v]: %v", sessionUsrName, message)
						if err := (*eachCliConn).WriteMessage(websocket.TextMessage, []byte(msgWithSession)); err != nil {
							fmt.Println(err)
							break
						}
					}
				}

			}
		}
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		file, err := os.Open("./views/index.html")
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)

		var html string = ""
		for scanner.Scan() {
			html += scanner.Text() + "\n"
		}
		fmt.Fprint(w, html)
	})

	fmt.Println("Server running on 3000")
	err := http.ListenAndServe("0.0.0.0:3000", nil)
	if err != nil {
		panic(err)
	}
}
